// Package web serves the embedded browser UI exclusively over loopback.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/NimbleMarkets/bells-deep/internal/gpu"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/preset"
	"github.com/NimbleMarkets/bells-deep/internal/preview"
	"github.com/NimbleMarkets/bells-deep/internal/session"
	"github.com/NimbleMarkets/bells-deep/internal/shader"
	assets "github.com/NimbleMarkets/bells-deep/web"
	"image/png"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type Server struct {
	URL     string
	http    *http.Server
	mu      sync.RWMutex
	metrics gpu.Metrics
	store   *session.Store
	thumbMu sync.Mutex
}

func (s *Server) Metrics(m gpu.Metrics) { s.mu.Lock(); s.metrics = m; s.mu.Unlock() }
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.http.Shutdown(ctx)
}
func Start(addr string, store *session.Store, fps, width, height int) (*Server, error) {
	host, _, e := net.SplitHostPort(addr)
	if e != nil {
		return nil, e
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("--web requires a numeric loopback address")
	}
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		return nil, e
	}
	s := &Server{URL: "http://" + ln.Addr().String() + "/", store: store}
	mux := http.NewServeMux()
	jsonOut := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		m := s.metrics
		s.mu.RUnlock()
		jsonOut(w, struct {
			session.Snapshot
			Metrics  gpu.Metrics       `json:"native"`
			Controls []mixture.Control `json:"controls"`
			FPS      int               `json:"target_fps"`
			Width    int               `json:"width"`
			Height   int               `json:"height"`
		}{store.Snapshot(), m, mixture.Controls, fps, width, height})
	})
	mux.HandleFunc("GET /api/shader", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, shader.BuildWGSL())
	})
	mux.HandleFunc("POST /api/action", func(w http.ResponseWriter, r *http.Request) {
		var a struct {
			Command   string          `json:"command"`
			Scene     *mixture.Scene  `json:"scene"`
			Camera    *mixture.Camera `json:"camera"`
			Toggle    *uint32         `json:"toggle"`
			RandomAll bool            `json:"random_all"`
		}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&a); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		if a.Scene != nil {
			store.Replace(*a.Scene)
		}
		if a.Camera != nil {
			store.Camera(*a.Camera)
		}
		if a.Toggle != nil && *a.Toggle < 5 {
			store.Edit(func(p *mixture.Scene) { p.Overlays ^= 1 << *a.Toggle })
		}
		if a.RandomAll {
			store.Edit(func(p *mixture.Scene) {
				for i := range p.Components {
					p.Randomize(i)
				}
			})
		}
		msg, e := store.Command(a.Command, false)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		jsonOut(w, struct {
			session.Snapshot
			Message string `json:"message"`
		}{store.Snapshot(), msg})
	})
	mux.HandleFunc("GET /api/uniform", func(w http.ResponseWriter, r *http.Request) {
		wth, _ := strconv.Atoi(r.URL.Query().Get("w"))
		h, _ := strconv.Atoi(r.URL.Query().Get("h"))
		if wth < 1 || h < 1 || wth > 4096 || h > 4096 {
			http.Error(w, "dimensions must be 1..4096", 400)
			return
		}
		v := store.Snapshot()
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(shader.Encode(v.Scene, v.Stats, wth, h, 0, 0))
	})
	mux.HandleFunc("GET /api/samples", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(gpu.SampleBytes(mixture.SampleCloud(store.Snapshot().Scene)))
	})
	type entry struct {
		Name    string `json:"name"`
		ID      string `json:"id"`
		Builtin bool   `json:"builtin"`
	}
	entries := func() []entry {
		out := []entry{}
		for _, n := range preset.Names {
			out = append(out, entry{n, "builtin:" + n, true})
		}
		for _, p := range preset.List(store.Gallery) {
			n := filepath.Base(p)
			out = append(out, entry{n, "file:" + n, false})
		}
		return out
	}
	load := func(id string) (mixture.Scene, error) {
		for _, v := range entries() {
			if v.ID == id {
				if v.Builtin {
					return preset.Builtin(v.Name)
				}
				return preset.Load(filepath.Join(store.Gallery, v.Name))
			}
		}
		return mixture.Default(), fmt.Errorf("unknown gallery entry")
	}
	mux.HandleFunc("GET /api/gallery", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, entries()) })
	mux.HandleFunc("GET /api/preset", func(w http.ResponseWriter, r *http.Request) {
		p, e := load(r.URL.Query().Get("id"))
		if e != nil {
			http.Error(w, e.Error(), 404)
			return
		}
		jsonOut(w, p)
	})
	mux.HandleFunc("GET /api/thumbnail", func(w http.ResponseWriter, r *http.Request) {
		p, e := load(r.URL.Query().Get("id"))
		if e != nil {
			http.Error(w, e.Error(), 404)
			return
		}
		s.thumbMu.Lock()
		defer s.thumbMu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, preview.CameraThumbnail(p, 160, 96))
	})
	mux.Handle("/", http.FileServer(http.FS(assets.Files)))
	// Reject DNS rebinding and cross-origin writes even on the local interface.
	s.http = &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != ln.Addr().String() {
			http.Error(w, "invalid host", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host || u.Scheme != "http" {
				http.Error(w, "invalid origin", 403)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob:; connect-src 'self'")
		mux.ServeHTTP(w, r)
	})}
	go func() { _ = s.http.Serve(ln) }()
	return s, nil
}

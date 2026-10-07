package web

import (
	"bytes"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/session"
	"github.com/NimbleMarkets/bells-deep/internal/shader"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLoopbackAndAssets(t *testing.T) {
	store := session.New(mixture.Default(), t.TempDir())
	if _, e := Start("0.0.0.0:0", store, 60, 1280, 720); e == nil {
		t.Fatal("accepted public bind")
	}
	s, e := Start("127.0.0.1:0", store, 60, 1280, 720)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, path := range []string{"", "app.js", "app.css", "api/state", "api/gallery", "api/thumbnail?id=builtin:isotropic"} {
		r, e := http.Get(s.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatal(path, r.Status)
		}
	}
	r, e := http.Get(s.URL + "api/shader")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if string(b) != shader.BuildWGSL() {
		t.Fatal("divergent shader")
	}
	r, e = http.Post(s.URL+"api/action", "application/json", strings.NewReader(`{"command":"/set rho 20"}`))
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if store.Snapshot().Scene.Components[0].Rho != .97 {
		t.Fatal("browser bypassed clamp")
	}
	r, e = http.Get(s.URL + "api/uniform?w=160&h=96")
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(r.Body)
	r.Body.Close()
	snap := store.Snapshot()
	if !bytes.Equal(b, shader.Encode(snap.Scene, snap.Stats, 160, 96, 0, 0)) {
		t.Fatal("different uniform bytes")
	}
	req, _ := http.NewRequest("POST", s.URL+"api/action", strings.NewReader(`{"command":"/delete"}`))
	req.Header.Set("Origin", "https://example.org")
	r, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("cross origin accepted")
	}
	r, e = http.Post(s.URL+"api/action", "application/json", strings.NewReader(`{"command":"/save surprise"}`))
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 400 {
		t.Fatal("browser save accepted")
	}
}

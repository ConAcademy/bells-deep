// Package session shares validated interaction and snapshots across the TUI and HTTP UI.
package session

import (
	"fmt"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/preset"
	"strconv"
	"strings"
	"sync"
)

type Snapshot struct {
	Scene    mixture.Scene `json:"scene"`
	Stats    mixture.Stats `json:"stats"`
	Selected int           `json:"selected"`
	Paused   bool          `json:"paused"`
	Revision uint64        `json:"revision"`
}
type Store struct {
	mu      sync.RWMutex
	s       Snapshot
	Gallery string
}

func New(s mixture.Scene, dir string) *Store {
	m := &Store{Gallery: dir}
	m.s.Scene = s.Clone()
	m.s.Stats = mixture.Analyze(s)
	return m
}
func (m *Store) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v := m.s
	v.Scene = v.Scene.Clone()
	v.Stats.Ellipses = append([]mixture.Ellipse(nil), v.Stats.Ellipses...)
	return v
}
func (m *Store) Edit(fn func(*mixture.Scene)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.s.Scene)
	mixture.Clamp(&m.s.Scene)
	m.s.Selected = min(m.s.Selected, len(m.s.Scene.Components)-1)
	m.s.Stats = mixture.Analyze(m.s.Scene)
	m.s.Revision++
}
func (m *Store) Camera(c mixture.Camera) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Scene.Camera = c
	mixture.Clamp(&m.s.Scene)
}
func (m *Store) Replace(s mixture.Scene) { m.Edit(func(dst *mixture.Scene) { *dst = s.Clone() }) }
func (m *Store) Select(i int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.s.Scene.Components)
	m.s.Selected = ((i % n) + n) % n
}
func (m *Store) Pause(v bool) { m.mu.Lock(); defer m.mu.Unlock(); m.s.Paused = v }
func (m *Store) Command(line string, allowFiles bool) (string, error) {
	a := strings.Fields(strings.TrimPrefix(line, "/"))
	if len(a) == 0 {
		return "", nil
	}
	s := m.Snapshot()
	need := func(n int) bool { return len(a) == n }
	switch a[0] {
	case "help", "gallery", "fullscreen", "quit":
		return a[0], nil
	case "pause":
		m.Pause(true)
	case "play":
		m.Pause(false)
	case "reset":
		m.Edit(func(s *mixture.Scene) { s.Home() })
	case "component":
		if !need(2) {
			return "", fmt.Errorf("usage: /component N")
		}
		i, e := strconv.Atoi(a[1])
		if e != nil || i < 1 || i > len(s.Scene.Components) {
			return "", fmt.Errorf("component must be 1..%d", len(s.Scene.Components))
		}
		m.Select(i - 1)
	case "add":
		m.Edit(func(p *mixture.Scene) { p.Add(s.Selected) })
		m.Select(len(m.Snapshot().Scene.Components) - 1)
	case "delete":
		m.Edit(func(p *mixture.Scene) { p.Delete(s.Selected) })
	case "randomize":
		m.Edit(func(p *mixture.Scene) { p.Randomize(s.Selected) })
	case "set":
		if !need(3) {
			return "", fmt.Errorf("usage: /set rho 0.8")
		}
		v, e := strconv.ParseFloat(a[2], 64)
		if e != nil {
			return "", e
		}
		ok := false
		m.Edit(func(p *mixture.Scene) { ok = p.Set(s.Selected, a[1], v) })
		if !ok {
			return "", fmt.Errorf("unknown control %q", a[1])
		}
	case "preset":
		if !need(2) {
			return "", fmt.Errorf("usage: /preset correlated")
		}
		p, e := preset.Builtin(a[1])
		if e != nil {
			return "", e
		}
		m.Replace(p)
	case "save":
		if !allowFiles {
			return "", fmt.Errorf("browser gallery is read-only")
		}
		if !need(2) {
			return "", fmt.Errorf("usage: /save name")
		}
		return "saved " + a[1], preset.Save(m.Gallery, a[1], s.Scene)
	case "load":
		if !allowFiles {
			return "", fmt.Errorf("use the browser gallery")
		}
		if !need(2) {
			return "", fmt.Errorf("usage: /load file.bell.json")
		}
		p, e := preset.Load(a[1])
		if e != nil {
			return "", e
		}
		m.Replace(p)
	default:
		return "", fmt.Errorf("unknown command /%s", a[0])
	}
	return "", nil
}

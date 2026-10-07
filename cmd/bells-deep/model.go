package main

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/bells-deep/internal/gpu"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/preset"
	"github.com/NimbleMarkets/bells-deep/internal/preview"
	"github.com/NimbleMarkets/bells-deep/internal/session"
	"github.com/NimbleMarkets/bells-deep/internal/shader"
	webui "github.com/NimbleMarkets/bells-deep/internal/web"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"image"
	"math"
	"path/filepath"
	"strings"
	"time"
)

type tickMsg time.Time
type frameMsg struct {
	img        *image.NRGBA
	metrics    gpu.Metrics
	err        error
	elapsed    time.Duration
	generation uint64
}
type thumbMsg struct {
	index      int
	img        *image.NRGBA
	generation uint64
}
type galleryItem struct {
	name, path string
	scene      mixture.Scene
	thumb      *image.NRGBA
}
type model struct {
	store                              *session.Store
	gpu                                *gpu.Executor
	server                             *webui.Server
	input                              textinput.Model
	prompt, fullscreen, busy, dirty    bool
	pic, galleryPic                    picture.Model
	cols, rows, focus, fps, maxW, maxH int
	status, dialog                     string
	gallery                            []galleryItem
	galleryIndex                       int
	generation                         uint64
	galleryGeneration                  uint64
	metrics                            gpu.Metrics
	last                               time.Time
	clock                              float32
	frame                              uint32
	velocity                           [2]float64
	drag                               bool
	mouseX, mouseY                     int
	sampleRevision                     uint64
	sampleBytes                        []byte
	revision                           uint64
}

func newModel(store *session.Store, exec *gpu.Executor, fps, w, h int) *model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "/ command"
	return &model{store: store, gpu: exec, input: in, fps: fps, maxW: w, maxH: h, pic: picture.NewWithConfig(picture.Config{KittyID: 43, Fit: picture.FitFill}), galleryPic: picture.NewWithConfig(picture.Config{KittyID: 44, Fit: picture.FitContain}), cols: 100, rows: 34, dirty: true, status: "↑↓ control · ←→ adjust · drag / hjkl orbit · / command · ? help"}
}
func (m *model) Init() tea.Cmd { return tea.Batch(m.pic.Init(), m.resizePicture(), m.tick()) }
func (m *model) tick() tea.Cmd {
	return tea.Tick(time.Second/time.Duration(m.fps), func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m *model) viewport() (int, int) { l := m.layout(); return l.pictureW, l.pictureH }
func (m *model) render() tea.Cmd {
	snap := m.store.Snapshot()
	w, h := m.dimensions()
	if m.sampleBytes == nil || m.sampleRevision != snap.Revision {
		m.sampleBytes = gpu.SampleBytes(mixture.SampleCloud(snap.Scene))
		m.sampleRevision = snap.Revision
	}
	samples := m.sampleBytes
	uniform := shader.Encode(snap.Scene, snap.Stats, w, h, m.clock, m.frame)
	m.frame++
	m.busy = true
	m.dirty = false
	generation := m.generation
	exec := m.gpu
	return func() tea.Msg {
		start := time.Now()
		var img *image.NRGBA
		var metrics gpu.Metrics
		var err error
		if exec != nil {
			img, metrics, err = exec.Render(uniform, samples, w, h)
		} else {
			img = preview.Thumbnail(snap.Scene, w, h)
		}
		return frameMsg{img, metrics, err, time.Since(start), generation}
	}
}
func (m *model) command(line string) tea.Cmd {
	msg, err := m.store.Command(line, true)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	m.dirty = true
	if msg == "quit" {
		return tea.Sequence(m.clearPictures(), tea.Quit)
	}
	switch msg {
	case "gallery":
		return m.openGallery()
	case "help":
		m.dialog = "help"
		m.generation++
		return m.clearPictures()
	case "fullscreen":
		m.fullscreen = !m.fullscreen
		return m.resizePicture()
	default:
		if msg != "" {
			m.status = msg
		}
	}
	return nil
}
func (m *model) openGallery() tea.Cmd {
	m.dialog = "gallery"
	m.galleryGeneration++
	m.generation++
	m.gallery = nil
	m.galleryIndex = 0
	for _, n := range preset.Names {
		s, _ := preset.Builtin(n)
		m.gallery = append(m.gallery, galleryItem{name: n, scene: s})
	}
	for _, p := range preset.List(m.store.Gallery) {
		s, e := preset.Load(p)
		if e == nil {
			m.gallery = append(m.gallery, galleryItem{name: filepath.Base(p), path: p, scene: s})
		}
	}
	return tea.Batch(m.clearPictures(), m.thumbnail(0))
}
func (m *model) thumbnail(i int) tea.Cmd {
	if i >= len(m.gallery) {
		return nil
	}
	s := m.gallery[i].scene
	g := m.galleryGeneration
	exec := m.gpu
	return func() tea.Msg {
		var img *image.NRGBA
		if exec != nil {
			img, _, _ = exec.Render(shader.Encode(s, mixture.Analyze(s), 160, 96, 0, 0), gpu.SampleBytes(mixture.SampleCloud(s)), 160, 96)
		}
		if img == nil {
			img = preview.CameraThumbnail(s, 160, 96)
		}
		return thumbMsg{i, img, g}
	}
}
func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.cols, m.rows = v.Width, v.Height
		m.input.SetWidth(max(5, v.Width-8))
		m.dirty = true
		m.generation++
		return m, m.resizePicture()
	case tickMsg:
		now := time.Time(v)
		dt := now.Sub(m.last).Seconds()
		if m.last.IsZero() {
			dt = 0
		}
		m.last = now
		s := m.store.Snapshot()
		if s.Revision != m.revision {
			m.revision = s.Revision
			m.dirty = true
		}
		if !s.Paused {
			m.clock += float32(math.Min(dt, .1))
			if !m.drag && (math.Abs(m.velocity[0])+math.Abs(m.velocity[1]) > 1e-5) {
				c := s.Scene.Camera
				c.Yaw += m.velocity[0]
				c.Pitch += m.velocity[1]
				m.store.Camera(c)
				m.velocity[0] *= .9
				m.velocity[1] *= .9
				m.dirty = true
			}
		}
		var render tea.Cmd
		if m.rows > 2 && !m.busy && m.dialog == "" && (m.dirty || (!s.Paused && s.Scene.Overlays&8 != 0 && m.gpu != nil)) {
			render = m.render()
		}
		return m, tea.Batch(m.tick(), render)
	case frameMsg:
		if v.err != nil {
			m.busy = false
			m.status = "GPU: " + v.err.Error()
			m.gpu = nil
			m.dirty = true
			return m, nil
		}
		if v.generation != m.generation || m.dialog != "" {
			m.busy = false
			m.dirty = true
			return m, nil
		}
		m.metrics = v.metrics
		if v.elapsed > 0 {
			m.metrics.FPS = math.Min(float64(m.fps), 1/v.elapsed.Seconds())
		}
		cmd := m.pic.SetImage(v.img)
		if cmd == nil {
			m.busy = false
			m.metrics.UploadBytes = len(m.pic.View().Content)
			m.metrics.Transport = "glyph"
			m.recordMetrics()
			return m, nil
		}
		generation := m.generation
		return m, func() tea.Msg { return encodedMsg{msg: cmd(), generation: generation} }
	case encodedMsg:
		// Always forward even a stale frame: NTCharts owns its buffer cleanup.
		cmd := m.pic.Update(v.msg)
		if v.generation == m.generation && m.dialog == "" {
			if frame, ok := v.msg.(picture.KittyFrameMsg); ok {
				m.metrics.UploadBytes = len(frame.APC)
				m.metrics.Transport = frame.Format.String()
				if frame.Medium == picture.KittyMediumSharedMemory {
					m.metrics.Transport = "shm"
				}
				m.recordMetrics()
			}
		}
		// Keep one native render/encode/upload in flight through grid placement.
		return m, tea.Sequence(cmd, func() tea.Msg { return presentedMsg{} })
	case presentedMsg:
		m.busy = false
		return m, nil
	case thumbMsg:
		if m.dialog != "gallery" || v.generation != m.galleryGeneration {
			return m, nil
		}
		if v.index < len(m.gallery) {
			m.gallery[v.index].thumb = v.img
		}
		var display tea.Cmd
		if v.index == m.galleryIndex {
			display = m.showGalleryThumbnail()
		}
		return m, tea.Batch(display, m.thumbnail(v.index+1))
	case tea.MouseClickMsg:
		l := m.layout()
		if m.dialog == "" && v.X >= l.pictureX && v.X < l.pictureX+l.pictureW && v.Y >= l.pictureY && v.Y < l.pictureY+l.pictureH {
			m.drag = true
			m.mouseX, m.mouseY = v.X, v.Y
		}
	case tea.MouseReleaseMsg:
		m.drag = false
	case tea.MouseMotionMsg:
		if m.drag && m.dialog == "" {
			m.velocity = [2]float64{float64(v.X-m.mouseX) * .025, float64(m.mouseY-v.Y) * .025}
			s := m.store.Snapshot()
			c := s.Scene.Camera
			c.Yaw += m.velocity[0]
			c.Pitch += m.velocity[1]
			m.store.Camera(c)
			m.mouseX, m.mouseY = v.X, v.Y
			m.dirty = true
		}
	case tea.MouseWheelMsg:
		s := m.store.Snapshot()
		c := s.Scene.Camera
		if strings.Contains(v.String(), "up") {
			c.Radius *= .9
		} else {
			c.Radius *= 1.1
		}
		m.store.Camera(c)
		m.dirty = true
	case tea.KeyPressMsg:
		key := v.String()
		if key == "ctrl+c" {
			return m, tea.Sequence(m.clearPictures(), tea.Quit)
		}
		if m.prompt {
			switch key {
			case "esc":
				m.prompt = false
				m.input.Blur()
				return m, nil
			case "enter":
				line := m.input.Value()
				m.input.SetValue("")
				m.input.Blur()
				m.prompt = false
				return m, m.command(line)
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.dialog != "" {
			switch key {
			case "esc", "q", "f1", "?":
				m.dialog = ""
				m.dirty = true
				m.generation++
			case "up":
				m.galleryIndex = max(0, m.galleryIndex-1)
			case "down":
				m.galleryIndex = min(len(m.gallery)-1, m.galleryIndex+1)
			case "enter":
				if m.dialog == "gallery" && len(m.gallery) > 0 {
					m.store.Replace(m.gallery[m.galleryIndex].scene)
					m.dialog = ""
					m.dirty = true
					m.generation++
				}
			}
			if m.dialog == "gallery" {
				return m, m.showGalleryThumbnail()
			}
			return m, tea.Batch(m.galleryPic.SetImage(nil), m.resizePicture())
		}
		s := m.store.Snapshot()
		switch key {
		case "q":
			return m, tea.Sequence(m.clearPictures(), tea.Quit)
		case "/":
			m.prompt = true
			m.input.SetValue("/")
			return m, m.input.Focus()
		case "up":
			m.focus = (m.focus + len(mixture.Controls) - 1) % len(mixture.Controls)
		case "down":
			m.focus = (m.focus + 1) % len(mixture.Controls)
		case "left", "right", "shift+left", "shift+right":
			selected := s.Selected
			p := mixture.Controls[m.focus]
			step := p.Step
			if strings.HasPrefix(key, "shift+") {
				step *= 10
			}
			if strings.HasSuffix(key, "left") {
				step = -step
			}
			m.store.Edit(func(s *mixture.Scene) { s.Set(selected, p.Name, s.Values(selected)[m.focus]+step) })
			m.dirty = true
		case "[":
			m.store.Select(s.Selected - 1)
		case "]":
			m.store.Select(s.Selected + 1)
		case "n":
			return m, m.command("/add")
		case "x":
			return m, m.command("/delete")
		case "r":
			return m, m.command("/randomize")
		case "R":
			m.store.Edit(func(p *mixture.Scene) {
				for i := range p.Components {
					p.Randomize(i)
				}
			})
			m.dirty = true
		case "space":
			m.store.Pause(!s.Paused)
		case "0":
			m.velocity = [2]float64{}
			return m, m.command("/reset")
		case "home":
			m.store.Edit(func(p *mixture.Scene) { p.Fit() })
			m.dirty = true
		case "f", "ctrl+f":
			m.fullscreen = !m.fullscreen
			m.generation++
			m.dirty = true
			return m, m.resizePicture()
		case "esc":
			m.fullscreen = false
			m.generation++
			m.dirty = true
			return m, m.resizePicture()
		case "1", "2", "3", "4", "5":
			m.store.Edit(func(p *mixture.Scene) { p.Overlays ^= 1 << uint(key[0]-'1') })
			m.dirty = true
		case "f3":
			return m, m.openGallery()
		case "f1", "?":
			return m, m.command("/help")
		case "h", "j", "k", "l", "+", "-":
			c := s.Scene.Camera
			switch key {
			case "h":
				m.velocity[0] = -.035
			case "l":
				m.velocity[0] = .035
			case "j":
				m.velocity[1] = -.025
			case "k":
				m.velocity[1] = .025
			case "+":
				c.Radius *= .9
			case "-":
				c.Radius *= 1.1
			}
			c.Yaw += m.velocity[0]
			c.Pitch += m.velocity[1]
			m.store.Camera(c)
			m.dirty = true
		}
		return m, nil
	}
	return m, nil
}

var purple = lipgloss.NewStyle().Foreground(lipgloss.Color("#BD9BDD"))
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#7E8DA6"))

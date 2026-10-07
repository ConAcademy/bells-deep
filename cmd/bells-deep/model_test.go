package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/session"
	"github.com/charmbracelet/x/ansi"

	"image"
	"strings"
	"testing"
)

func TestKeys(t *testing.T) {
	s := session.New(mixture.Default(), t.TempDir())
	m := newModel(s, nil, 60, 1280, 720)
	m.focus = 5
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.Snapshot().Scene.Components[0].Rho != .01 {
		t.Fatal("right arrow")
	}
	m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if len(s.Snapshot().Scene.Components) != 2 {
		t.Fatal("add")
	}
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if s.Snapshot().Scene.Overlays&1 != 0 {
		t.Fatal("grid toggle")
	}
	if !m.View().AltScreen || !strings.Contains(m.View().Content, "peak") {
		t.Fatal("missing HUD")
	}
}

func TestLayoutNeverOverflows(t *testing.T) {
	sizes := [][2]int{{1, 1}, {12, 5}, {31, 13}, {32, 14}, {50, 18}, {80, 24}, {84, 24}, {85, 24}, {100, 24}, {100, 34}, {160, 50}}
	for _, kitty := range []bool{false, true} {
		for _, full := range []bool{false, true} {
			for _, size := range sizes {
				m := newModel(session.New(mixture.Default(), t.TempDir()), nil, 60, 1280, 720)
				setCapability(t, kitty)
				drain(t, m, m.resizePicture())
				_, startup := m.Update(nil)
				drain(t, m, startup)
				m.fullscreen = full
				m.cols, m.rows = size[0], size[1]
				// Simulate a frame still at the old size while a resize is in flight.
				m.pic.SetCellPixelSize(1, 2)
				m.pic.SetSize(120, 24)
				drain(t, m, m.pic.SetImage(image.NewNRGBA(image.Rect(0, 0, 120, 48))))
				for _, dialog := range []string{"", "help", "gallery"} {
					m.dialog = dialog
					v := m.View()
					lines := strings.Split(v.Content, "\n")
					if len(lines) > m.rows {
						t.Fatalf("%v kitty=%v full=%v dialog=%s: %d rows", size, kitty, full, dialog, len(lines))
					}
					for _, line := range lines {
						if w := ansi.StringWidth(line); w > m.cols {
							t.Fatalf("%v: line width %d", size, w)
						}
					}
				}
				l := m.layout()
				if size[1] >= 5 && l.pictureY+l.pictureH > m.rows-2 {
					t.Fatalf("%v: image overlaps footer: %+v", size, l)
				}
				if l.pictureX+l.pictureW > m.cols {
					t.Fatalf("%v: image exceeds right edge: %+v", size, l)
				}
			}
		}
	}
}

func TestResizeDiscardsOldFrame(t *testing.T) {
	m := newModel(session.New(mixture.Default(), t.TempDir()), nil, 60, 1280, 720)
	m.busy = true
	old := m.generation
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 18})
	_, cmd := m.Update(frameMsg{img: image.NewNRGBA(image.Rect(0, 0, 160, 96)), generation: old})
	if cmd != nil || m.pic.View().Content != "" || !m.dirty || m.busy {
		t.Fatal("stale frame displayed after resize")
	}
}

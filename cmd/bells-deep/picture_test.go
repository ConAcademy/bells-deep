package main

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/base64"
	"fmt"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/session"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"
)

func setCapability(t *testing.T, supported bool) {
	t.Helper()
	old := picture.KittySupported()
	t.Cleanup(func() { picture.ForceKittyCapability(old) })
	c := picture.KittyCapabilityUnsupported
	if supported {
		c = picture.KittyCapabilitySupported
	}
	picture.ForceKittyCapability(c)
}

// Execute finite widget commands and feed their messages through our real Update.
// Tea's batch/sequence messages are slices of Cmd; no event-loop internals are copied.
func drain(t *testing.T, m *model, cmd tea.Cmd) (raw []string) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	v := reflect.ValueOf(msg)
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			if c, ok := v.Index(i).Interface().(tea.Cmd); ok {
				raw = append(raw, drain(t, m, c)...)
			} else {
				t.Fatalf("unexpected command group %T", msg)
			}
		}
		return
	}
	if r, ok := msg.(tea.RawMsg); ok {
		return []string{fmt.Sprint(r.Msg)}
	}
	_, next := m.Update(msg)
	return drain(t, m, next)
}
func testModel(t *testing.T) *model {
	t.Helper()
	m := newModel(session.New(mixture.Default(), t.TempDir()), nil, 60, 1280, 720)
	drain(t, m, m.resizePicture())
	t.Cleanup(func() { m.clearPictures() })
	return m
}

func TestTerminalResponseSelectsNTChartsKitty(t *testing.T) {
	setCapability(t, false)
	picture.ForceKittyCapability(picture.KittyCapabilityUnknown)
	m := testModel(t)
	if m.pic.Mode() != picture.PictureGlyph {
		t.Fatal("should start with safe glyph fallback")
	}
	_, cmd := m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 42069101}, Payload: []byte("OK")})
	drain(t, m, cmd)
	if m.pic.Mode() != picture.PictureKitty || m.galleryPic.Mode() != picture.PictureKitty {
		t.Fatal("probe reply not forwarded to NTCharts")
	}
	_, cmd = m.Update(uv.CellSizeEvent{Width: 13, Height: 27})
	drain(t, m, cmd)
	w, h := m.pic.CellPixelSize()
	if w != 13 || h != 27 {
		t.Fatal("cell size not forwarded")
	}
}

func TestRasterMatchesNTChartsUpload(t *testing.T) {
	setCapability(t, true)
	m := testModel(t)
	_, cmd := m.Update(uv.CellSizeEvent{Width: 13, Height: 27})
	drain(t, m, cmd)
	m.cols, m.rows = 180, 55
	drain(t, m, m.resizePicture())
	w, h := m.dimensions()
	frame := m.pic.SetImage(image.NewNRGBA(image.Rect(0, 0, w, h)))().(picture.KittyFrameMsg)
	var payload strings.Builder
	for _, chunk := range strings.Split(frame.APC, "\x1b\\") {
		if strings.HasPrefix(chunk, "\x1b_G") {
			_, data, ok := strings.Cut(chunk, ";")
			if ok {
				payload.WriteString(data)
			}
		}
	}
	data, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != w || cfg.Height != h {
		t.Fatalf("GPU %dx%d was rescaled to %dx%d", w, h, cfg.Width, cfg.Height)
	}
	if w > m.maxW || h > m.maxH {
		t.Fatalf("exceeded raster budget %dx%d", w, h)
	}
}

func TestNTChartsDiscardsEncodedFrameAfterResize(t *testing.T) {
	setCapability(t, true)
	m := testModel(t)
	_, cmd := m.Update(nil)
	drain(t, m, cmd)
	w, h := m.dimensions()
	m.busy = true
	_, encode := m.Update(frameMsg{img: image.NewNRGBA(image.Rect(0, 0, w, h)), generation: m.generation})
	encoded := encode()
	_, resize := m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	drain(t, m, resize)
	_, present := m.Update(encoded)
	raw := strings.Join(drain(t, m, present), "")
	if strings.Contains(raw, "a=T") {
		t.Fatal("stale frame uploaded after resize")
	}
	if m.busy || m.pic.View().Content != "" {
		t.Fatal("stale frame remained visible or pipeline stuck")
	}
}

func TestNativePresentationCompletesBeforeNextFrame(t *testing.T) {
	setCapability(t, true)
	m := testModel(t)
	_, cmd := m.Update(nil)
	drain(t, m, cmd)
	w, h := m.dimensions()
	m.busy = true
	_, cmd = m.Update(frameMsg{img: image.NewNRGBA(image.Rect(0, 0, w, h)), generation: m.generation})
	if !m.busy {
		t.Fatal("GPU slot released before encoding")
	}
	raw := strings.Join(drain(t, m, cmd), "")
	if !strings.Contains(raw, "a=T") || m.pic.View().Content == "" || m.busy {
		t.Fatal("upload/grid lifecycle did not complete")
	}
	if m.metrics.Transport != "png" || m.metrics.UploadBytes == 0 {
		t.Fatal(m.metrics)
	}
}

func TestGalleryUsesSeparatePicture(t *testing.T) {
	setCapability(t, true)
	m := testModel(t)
	_, cmd := m.Update(nil)
	drain(t, m, cmd)
	w, h := m.dimensions()
	drain(t, m, m.pic.SetImage(image.NewNRGBA(image.Rect(0, 0, w, h))))
	drain(t, m, m.openGallery())
	if m.pic.View().Content != "" || m.galleryPic.View().Content == "" {
		t.Fatal("gallery should replace the main picture with its own widget")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	drain(t, m, cmd)
	if m.galleryPic.View().Content != "" || !m.dirty {
		t.Fatal("gallery image was not cleared")
	}
}

func TestKittyTransportSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		env    map[string]string
		shared bool
		want   picture.KittyMedium
	}{{"auto", map[string]string{"TERM": "xterm-ghostty"}, true, picture.KittyMediumSharedMemory}, {"auto", map[string]string{"TERM": "xterm-ghostty", "SSH_TTY": "/dev/pts/1"}, true, picture.KittyMediumDirect}, {"auto", map[string]string{"TERM": "xterm-ghostty"}, false, picture.KittyMediumDirect}, {"direct", nil, true, picture.KittyMediumDirect}} {
		got, err := kittyMedium(tc.name, func(k string) string { return tc.env[k] }, tc.shared)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %v %v", tc, got, err)
		}
	}
	if _, err := kittyMedium("bad", func(string) string { return "" }, true); err == nil {
		t.Fatal("invalid transport accepted")
	}
}

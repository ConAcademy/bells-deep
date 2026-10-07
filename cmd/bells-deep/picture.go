package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	uv "github.com/charmbracelet/ultraviolet"
	"math"
)

type encodedMsg struct {
	msg        tea.Msg
	generation uint64
}
type presentedMsg struct{}

// Update forwards terminal capability, cell-size and asynchronous picture
// messages to both widgets. NTCharts filters each frame by widget identity.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if size, ok := msg.(uv.CellSizeEvent); ok {
		w, h := m.pic.CellPixelSize()
		if w != size.Width || h != size.Height {
			cmds = append(cmds, m.clearPictures())
			m.generation++
			m.dirty = true
		}
	}
	cmds = append(cmds, m.pic.Update(msg), m.galleryPic.Update(msg))
	if picture.KittySupported() == picture.KittyCapabilitySupported && m.pic.Mode() == picture.PictureGlyph {
		// Discard the low-resolution fallback before switching, avoiding a CPU upscale.
		cmds = append(cmds, m.clearPictures(), m.pic.Toggle(), m.galleryPic.Toggle())
		cmds = append(cmds, m.resizePicture())
	}
	if _, ok := msg.(uv.CellSizeEvent); ok {
		cmds = append(cmds, m.resizePicture())
	}
	_, cmd := m.update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// Match NTCharts' integer effective-cell calculation exactly. The budget is
// soft: a one-pixel cell is the minimum upload resolution.
func rasterSize(cols, rows, cw, ch, maxW, maxH int) (factor float64, w, h int) {
	cols, rows, cw, ch = max(1, cols), max(1, rows), max(1, cw), max(1, ch)
	factor = math.Min(1, math.Min(float64(maxW)/float64(cols*cw), float64(maxH)/float64(rows*ch)))
	return factor, cols * max(1, int(float64(cw)*factor)), rows * max(1, int(float64(ch)*factor))
}
func (m *model) dimensions() (int, int) {
	cols, rows := m.viewport()
	if m.pic.Mode() == picture.PictureKitty {
		cw, ch := m.pic.CellPixelSize()
		_, w, h := rasterSize(cols, rows, cw, ch, m.maxW, m.maxH)
		return w, h
	}
	return min(cols, m.maxW), min(rows*2, m.maxH)
}
func (m *model) resizePicture() tea.Cmd {
	m.generation++
	m.dirty = true
	clear := m.pic.SetImage(nil)
	cols, rows := m.viewport()
	if m.rows <= 2 {
		cols, rows = 0, 0
	}
	size := m.pic.SetSize(cols, rows)
	cw, ch := m.pic.CellPixelSize()
	factor, _, _ := rasterSize(cols, rows, cw, ch, m.maxW, m.maxH)
	resolution := m.pic.SetKittyResolutionFactor(factor)
	return tea.Batch(clear, size, resolution, m.showGalleryThumbnail())
}
func (m *model) showGalleryThumbnail() tea.Cmd {
	if m.dialog != "gallery" || len(m.gallery) == 0 {
		return m.galleryPic.SetImage(nil)
	}
	// The gallery list and footer consume all other rows; never encode a hidden preview.
	count := max(1, min(7, m.rows-8))
	start := max(0, m.galleryIndex-count/2)
	count = min(len(m.gallery)-start, count)
	rows := min(8, max(0, m.rows-2-(4+count)))
	clear := m.galleryPic.SetImage(nil)
	size := m.galleryPic.SetSize(min(30, max(0, m.cols)), rows)
	var display tea.Cmd
	if rows > 0 {
		if img := m.gallery[m.galleryIndex].thumb; img != nil {
			display = m.galleryPic.SetImage(img)
		}
	}
	return tea.Sequence(clear, size, display)
}
func (m *model) clearPictures() tea.Cmd {
	return tea.Batch(m.pic.SetImage(nil), m.galleryPic.SetImage(nil))
}
func (m *model) recordMetrics() {
	if m.server != nil {
		m.server.Metrics(m.metrics)
	}
}

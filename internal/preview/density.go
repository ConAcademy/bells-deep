// Package preview creates CPU analytic previews when a native GPU is unavailable.
package preview

import (
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"image"
	"image/color"
	"math"
)

// Thumbnail is an explicitly CPU-only analytic top-down density preview.
func Thumbnail(s mixture.Scene, w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	peak := mixture.Analyze(s).Peak[2]
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := (float64(x)+.5)/float64(w)*2*s.Domain-s.Domain, s.Domain-(float64(y)+.5)/float64(h)*2*s.Domain
			t := math.Pow(s.PDF(px, py)/peak, .6)
			t = math.Min(1, t)
			img.SetNRGBA(x, y, color.NRGBA{uint8(15 + 235*t), uint8(23 + 180*t), uint8(36 + 110*t), 255})
		}
	}
	return img
}

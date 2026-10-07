package gpu

import (
	"encoding/binary"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/shader"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderIsotropic(t *testing.T) {
	e := New()
	defer e.Close()
	if err := e.Available(); err != nil {
		t.Skipf("no GPU adapter: %v", err)
	}
	s := mixture.Default()
	s.Camera.Pitch = math.Pi/2 - .001
	s.Camera.Target = [3]float64{}
	s.Camera.Yaw = 0
	s.Camera.Radius = 11
	s.Overlays = 0
	stats := mixture.Analyze(s)
	u := shader.Encode(s, stats, 96, 96, 0, 0)
	if len(u) != 256 || binary.LittleEndian.Uint32(u[32:]) != 96 {
		t.Fatal("uniform layout")
	}
	img, _, err := e.Render(u, SampleBytes(nil), 96, 96)
	if err != nil {
		t.Fatal(err)
	}
	best, x, y := -1, 0, 0
	for py := 0; py < 96; py++ {
		for px := 0; px < 96; px++ {
			c := img.NRGBAAt(px, py)
			v := int(c.R) + int(c.G) + int(c.B)
			if v > best {
				best, x, y = v, px, py
			}
		}
	}
	if math.Hypot(float64(x-48), float64(y-48)) > 12 {
		t.Fatalf("brightest pixel (%d,%d) = %d, not centered peak", x, y, best)
	}
	s = mixture.Default()
	u = shader.Encode(s, mixture.Analyze(s), 320, 192, 0, 0)
	img, _, err = e.Render(u, SampleBytes(mixture.SampleCloud(s)), 320, 192)
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("BELLS_TEST_RENDER_DIR"); path != "" {
		f, er := os.Create(filepath.Join(path, "bells-gpu.png"))
		if er != nil {
			t.Fatal(er)
		}
		defer f.Close()
		if er = png.Encode(f, img); er != nil {
			t.Fatal(er)
		}
	}
	if err = e.do(func(d *device) error {
		if len(d.cache) != 1 {
			t.Errorf("uniform change recompiled shader: %d", len(d.cache))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

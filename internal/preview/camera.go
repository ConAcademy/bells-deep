package preview

import (
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"image"
	"image/color"
	"math"
)

type vec [3]float64

func add(a, b vec) vec         { return vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func mul(a vec, t float64) vec { return vec{a[0] * t, a[1] * t, a[2] * t} }
func dot(a, b vec) float64     { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func unit(a vec) vec           { return mul(a, 1/math.Sqrt(dot(a, a))) }
func cross(a, b vec) vec {
	return vec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// CameraThumbnail is a small analytic fallback for gallery previews, including
// web-only sessions with no native device. It respects the saved scene camera.
func CameraThumbnail(s mixture.Scene, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	c := s.Camera
	target := vec(c.Target)
	r := c.Radius * math.Cos(c.Pitch)
	eye := add(target, vec{r * math.Cos(c.Yaw), r * math.Sin(c.Yaw), c.Radius * math.Sin(c.Pitch)})
	forward := unit(add(target, mul(eye, -1)))
	right := unit(cross(forward, vec{0, 0, 1}))
	up := cross(right, forward)
	peak := mixture.Analyze(s).Peak[2]
	scale := s.Height
	if s.Normalize {
		scale /= peak
	}
	height := func(p vec) float64 { return s.PDF(p[0], p[1]) * scale }
	light := unit(vec{-.5, -.7, 1.5})
	extent := s.Domain
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			u, v := (float64(x)+.5-float64(w)*.5)/float64(h), (float64(y)+.5-float64(h)*.5)/float64(h)
			ray := unit(add(forward, mul(add(mul(right, u), mul(up, -v)), 2*math.Tan(math.Pi/8))))
			near, far := 0., math.Inf(1)
			for i := 0; i < 3; i++ {
				lo, hi := -extent, extent
				if i == 2 {
					lo, hi = 0, peak*scale+.01
				}
				a, b := (lo-eye[i])/ray[i], (hi-eye[i])/ray[i]
				if a > b {
					a, b = b, a
				}
				near = math.Max(near, a)
				far = math.Min(far, b)
			}
			rgb := vec{.025, .035, .055}
			if far > near {
				step := (far - near) / 192
				last := near
				for j := 0; j <= 192; j++ {
					t := near + float64(j)*step
					p := add(eye, mul(ray, t))
					if p[2] <= height(p) {
						a, b := last, t
						for k := 0; k < 8; k++ {
							mid := (a + b) * .5
							q := add(eye, mul(ray, mid))
							if q[2] > height(q) {
								a = mid
							} else {
								b = mid
							}
						}
						p = add(eye, mul(ray, (a+b)*.5))
						eps := .002 * extent
						n := unit(vec{height(add(p, vec{-eps, 0, 0})) - height(add(p, vec{eps, 0, 0})), height(add(p, vec{0, -eps, 0})) - height(add(p, vec{0, eps, 0})), 2 * eps})
						z := math.Pow(math.Min(1, s.PDF(p[0], p[1])/peak), .6)
						rgb = mul(add(mul(vec{.13, .21, .32}, 1-z), mul(vec{.98, .82, .59}, z)), .55+.45*math.Max(0, dot(n, light)))
						break
					}
					last = t
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(rgb[0] * 255), uint8(rgb[1] * 255), uint8(rgb[2] * 255), 255})
		}
	}
	return out
}

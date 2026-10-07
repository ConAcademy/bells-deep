// Package mixture owns all parameters, constraints and CPU statistics.
package mixture

import (
	"math"
	"math/rand/v2"
)

type Component struct {
	Weight float64 `json:"weight"`
	MuX    float64 `json:"mu_x"`
	MuY    float64 `json:"mu_y"`
	SigmaX float64 `json:"sigma_x"`
	SigmaY float64 `json:"sigma_y"`
	Rho    float64 `json:"rho"`
	Color  uint32  `json:"color"`
}
type Camera struct {
	Yaw    float64    `json:"yaw"`
	Pitch  float64    `json:"pitch"`
	Radius float64    `json:"radius"`
	Target [3]float64 `json:"target"`
}
type Scene struct {
	Version    int         `json:"version"`
	Components []Component `json:"components"`
	Domain     float64     `json:"domain"`
	Height     float64     `json:"height"`
	Normalize  bool        `json:"normalize"`
	Samples    int         `json:"samples"`
	Seed       uint64      `json:"seed"`
	Overlays   uint32      `json:"overlays"`
	Camera     Camera      `json:"camera"`
}

var Colors = []uint32{0xf2b875, 0xcaa0de, 0x8dd6a4, 0x8ec4ed, 0xdea0a0, 0xb3c986, 0xd1b8f5, 0x75caca}

func DefaultComponent() Component {
	return Component{Weight: 1, SigmaX: 1, SigmaY: 1, Color: Colors[0]}
}
func Default() Scene {
	s := Scene{Version: 1, Components: []Component{DefaultComponent()}, Domain: 4, Height: 3, Normalize: true, Samples: 512, Seed: 42, Overlays: 7}
	s.Home()
	return s
}
func Limit(x, lo, hi float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return lo
	}
	return math.Max(lo, math.Min(hi, x))
}

// Clamp is the sole validation boundary for TUI, JSON and browser updates.
func Clamp(s *Scene) {
	s.Version = 1
	s.Domain = Limit(s.Domain, .5, 20)
	s.Height = Limit(s.Height, .1, 10)
	s.Samples = max(0, min(4096, s.Samples))
	s.Overlays &= 31
	if len(s.Components) == 0 {
		s.Components = []Component{DefaultComponent()}
	}
	if len(s.Components) > 8 {
		s.Components = s.Components[:8]
	}
	for i := range s.Components {
		c := &s.Components[i]
		c.Weight = Limit(c.Weight, .001, 10)
		c.MuX = Limit(c.MuX, -s.Domain, s.Domain)
		c.MuY = Limit(c.MuY, -s.Domain, s.Domain)
		c.SigmaX = Limit(c.SigmaX, .05, 4)
		c.SigmaY = Limit(c.SigmaY, .05, 4)
		c.Rho = Limit(c.Rho, -.97, .97)
		c.Color &= 0xffffff
	}
	s.Camera.Pitch = Limit(s.Camera.Pitch, .08, 1.55)
	s.Camera.Radius = Limit(s.Camera.Radius, 1, 100)
	s.Camera.Yaw = math.Remainder(Limit(s.Camera.Yaw, -1e6, 1e6), 2*math.Pi)
	for i := range s.Camera.Target {
		s.Camera.Target[i] = Limit(s.Camera.Target[i], -20, 20)
	}
}
func (s Scene) Clone() Scene { s.Components = append([]Component(nil), s.Components...); return s }
func (s *Scene) Home() {
	s.Camera = Camera{Yaw: -.8, Pitch: 35 * math.Pi / 180, Radius: s.Domain * 2.7, Target: [3]float64{0, 0, .6}}
}
func (s *Scene) Fit() {
	d := .5
	for _, c := range s.Components {
		d = math.Max(d, math.Max(math.Abs(c.MuX)+3*c.SigmaX, math.Abs(c.MuY)+3*c.SigmaY))
	}
	s.Domain = Limit(d, .5, 20)
	s.Home()
}
func Gaussian2(c Component, x, y float64) float64 {
	a, b := (x-c.MuX)/c.SigmaX, (y-c.MuY)/c.SigmaY
	v := 1 - c.Rho*c.Rho
	return math.Exp(-(a*a+b*b-2*c.Rho*a*b)/(2*v)) / (2 * math.Pi * c.SigmaX * c.SigmaY * math.Sqrt(v))
}
func (s Scene) WeightSum() float64 {
	v := 0.
	for _, c := range s.Components {
		v += c.Weight
	}
	return v
}
func (s Scene) PDF(x, y float64) float64 {
	v := 0.
	for _, c := range s.Components {
		v += c.Weight * Gaussian2(c, x, y)
	}
	return v / s.WeightSum()
}

type Ellipse struct {
	Det         float64    `json:"det"`
	Condition   float64    `json:"condition"`
	Eigenvalues [2]float64 `json:"eigenvalues"`
	Axes        [2]float64 `json:"axes"`
	Rotation    float64    `json:"rotation"`
}

func Covariance(c Component) Ellipse {
	a, d, b := c.SigmaX*c.SigmaX, c.SigmaY*c.SigmaY, c.Rho*c.SigmaX*c.SigmaY
	disc := math.Hypot(a-d, 2*b)
	hi := (a + d + disc) / 2
	det := a*d - b*b
	lo := det / hi
	return Ellipse{det, hi / lo, [2]float64{hi, lo}, [2]float64{math.Sqrt(hi), math.Sqrt(lo)}, .5 * math.Atan2(2*b, a-d)}
}

type Stats struct {
	Peak         [3]float64 `json:"peak"`
	Ellipses     []Ellipse  `json:"ellipses"`
	WeightBefore float64    `json:"weight_before"`
	WeightAfter  float64    `json:"weight_after"`
	Singular     bool       `json:"singular"`
	Warning      bool       `json:"warning"`
}

func Analyze(s Scene) Stats {
	r := Stats{WeightBefore: s.WeightSum(), WeightAfter: 1}
	seeds := [][2]float64{}
	for _, c := range s.Components {
		r.Ellipses = append(r.Ellipses, Covariance(c))
		r.Singular = r.Singular || 1-math.Abs(c.Rho) <= 1e-3
		r.Warning = r.Warning || math.Abs(c.Rho) >= .97
		seeds = append(seeds, [2]float64{c.MuX, c.MuY})
	}
	for y := 0; y <= 64; y++ {
		for x := 0; x <= 64; x++ {
			px, py := (float64(x)/32-1)*s.Domain, (float64(y)/32-1)*s.Domain
			z := s.PDF(px, py)
			if z > r.Peak[2] {
				r.Peak = [3]float64{px, py, z}
			}
		}
	}
	seeds = append(seeds, [2]float64{r.Peak[0], r.Peak[1]})
	for _, p := range seeds {
		step := s.Domain / 16
		z := s.PDF(p[0], p[1])
		for iter := 0; iter < 70; iter++ {
			found := false
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					x, y := Limit(p[0]+float64(dx)*step, -s.Domain, s.Domain), Limit(p[1]+float64(dy)*step, -s.Domain, s.Domain)
					v := s.PDF(x, y)
					if v > z {
						p = [2]float64{x, y}
						z = v
						found = true
					}
				}
			}
			if !found {
				step *= .5
			}
		}
		if z > r.Peak[2] {
			r.Peak = [3]float64{p[0], p[1], z}
		}
	}
	return r
}
func (s *Scene) Add(i int) {
	i = max(0, min(i, len(s.Components)-1))
	if len(s.Components) >= 8 {
		return
	}
	c := s.Components[i]
	c.MuX += .6
	c.MuY += .3
	c.Color = Colors[len(s.Components)]
	s.Components = append(s.Components, c)
	Clamp(s)
}
func (s *Scene) Delete(i int) {
	i = max(0, min(i, len(s.Components)-1))
	if len(s.Components) > 1 {
		s.Components = append(s.Components[:i], s.Components[i+1:]...)
	}
}
func (s *Scene) Randomize(i int) {
	i = max(0, min(i, len(s.Components)-1))
	c := &s.Components[i]
	c.Weight = .1 + rand.Float64()
	c.MuX = (rand.Float64()*2 - 1) * s.Domain * .6
	c.MuY = (rand.Float64()*2 - 1) * s.Domain * .6
	c.SigmaX = .3 + rand.Float64()*1.7
	c.SigmaY = .3 + rand.Float64()*1.7
	c.Rho = rand.Float64()*1.8 - .9
	Clamp(s)
}

type Control struct {
	Name    string  `json:"name"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Step    float64 `json:"step"`
	Default float64 `json:"default"`
}

var Controls = []Control{{"weight", .001, 10, .05, 1}, {"mu_x", -20, 20, .1, 0}, {"mu_y", -20, 20, .1, 0}, {"sigma_x", .05, 4, .05, 1}, {"sigma_y", .05, 4, .05, 1}, {"rho", -.97, .97, .01, 0}, {"domain", .5, 20, .1, 4}, {"height", .1, 10, .1, 3}, {"samples", 0, 4096, 64, 512}, {"normalize", 0, 1, 1, 1}}

func (s Scene) Values(i int) []float64 {
	c := s.Components[i]
	n := 0.
	if s.Normalize {
		n = 1
	}
	return []float64{c.Weight, c.MuX, c.MuY, c.SigmaX, c.SigmaY, c.Rho, s.Domain, s.Height, float64(s.Samples), n}
}
func (s *Scene) Set(i int, name string, v float64) bool {
	i = max(0, min(i, len(s.Components)-1))
	c := &s.Components[i]
	switch name {
	case "weight":
		c.Weight = v
	case "mu_x":
		c.MuX = v
	case "mu_y":
		c.MuY = v
	case "sigma_x":
		c.SigmaX = v
	case "sigma_y":
		c.SigmaY = v
	case "rho":
		c.Rho = v
	case "domain":
		s.Domain = v
	case "height":
		s.Height = v
	case "samples":
		s.Samples = int(v)
	case "normalize":
		s.Normalize = v >= .5
	default:
		return false
	}
	Clamp(s)
	return true
}

// SampleCloud is deterministic and regenerated only on parameter changes.
func SampleCloud(s Scene) [][4]float32 {
	r := rand.New(rand.NewPCG(s.Seed, 17))
	pts := make([][4]float32, s.Samples)
	sum := s.WeightSum()
	for i := range pts {
		u := r.Float64() * sum
		c := s.Components[len(s.Components)-1]
		for _, q := range s.Components {
			u -= q.Weight
			if u <= 0 {
				c = q
				break
			}
		}
		a, b := r.NormFloat64(), r.NormFloat64()
		pts[i] = [4]float32{float32(c.MuX + c.SigmaX*a), float32(c.MuY + c.SigmaY*(c.Rho*a+math.Sqrt(1-c.Rho*c.Rho)*b)), 0, float32(c.Color)}
	}
	return pts
}

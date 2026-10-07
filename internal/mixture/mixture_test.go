package mixture

import (
	"math"
	"reflect"
	"testing"
)

func TestGaussianAndPeak(t *testing.T) {
	s := Default()
	want := 1 / (2 * math.Pi)
	if math.Abs(s.PDF(0, 0)-want) > 1e-12 {
		t.Fatal(s.PDF(0, 0))
	}
	st := Analyze(s)
	if math.Abs(st.Peak[2]-want) > 1e-8 || math.Hypot(st.Peak[0], st.Peak[1]) > 1e-5 {
		t.Fatal(st.Peak)
	}
	s.Components[0].SigmaX = .05
	s.Components[0].SigmaY = .05
	s.Components[0].MuX = .723
	s.Components[0].MuY = -.632
	st = Analyze(s)
	if math.Hypot(st.Peak[0]-.723, st.Peak[1]+.632) > 1e-4 {
		t.Fatal("missed narrow peak", st.Peak)
	}
}
func TestPDFIntegratesToOne(t *testing.T) {
	s := Default()
	s.Components[0].Rho = .8
	s.Components[0].Weight = 3
	s.Add(0)
	s.Components[1].MuX = 1
	sum := 0.
	const step = .04
	for y := -8.; y < 8; y += step {
		for x := -8.; x < 8; x += step {
			sum += s.PDF(x, y) * step * step
		}
	}
	if math.Abs(sum-1) > 1e-5 {
		t.Fatal(sum)
	}
}
func TestCovarianceAndClamp(t *testing.T) {
	s := Default()
	c := &s.Components[0]
	c.Rho = .8
	e := Covariance(*c)
	if math.Abs(e.Det-.36) > 1e-12 || math.Abs(e.Condition-9) > 1e-12 || math.Abs(e.Rotation-math.Pi/4) > 1e-12 {
		t.Fatal(e)
	}
	c.Rho = 1
	c.SigmaX = 0
	c.MuX = 999
	c.Weight = -1
	s.Samples = 10000
	Clamp(&s)
	if c.Rho != .97 || c.SigmaX != .05 || c.MuX != s.Domain || c.Weight != .001 || s.Samples != 4096 {
		t.Fatal(s)
	}
	st := Analyze(s)
	if !st.Warning || st.Singular {
		t.Fatal(st)
	}
}
func TestSamplesSeededAndCorrelated(t *testing.T) {
	s := Default()
	s.Samples = 4096
	s.Components[0].Rho = .8
	a, b := SampleCloud(s), SampleCloud(s)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("unstable cloud")
	}
	var xx, yy, xy float64
	for _, p := range a {
		xx += float64(p[0] * p[0])
		yy += float64(p[1] * p[1])
		xy += float64(p[0] * p[1])
	}
	if math.Abs(xy/math.Sqrt(xx*yy)-.8) > .03 {
		t.Fatal("wrong covariance")
	}
}

package shader

import (
	"encoding/binary"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/gogpu/naga"
	"math"
	"testing"
)

func TestLayout(t *testing.T) {
	s := mixture.Default()
	s.Add(0)
	s.Components[1].MuX = 2
	s.Components[1].Rho = .8
	b := Encode(s, mixture.Analyze(s), 160, 96, 1.5, 7)
	if len(b) != 256 {
		t.Fatal(len(b))
	}
	u := func(o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
	if u(32) != 160 || u(36) != 96 || u(40) != 7 || u(60) != 2 {
		t.Fatal("header offsets")
	}
	if math.Float32frombits(u(92)) != 2 || math.Float32frombits(u(104)) != .8 || u(76) != 0x3c003c00 {
		t.Fatal("component offsets")
	}
}
func TestWGSL(t *testing.T) {
	ast, e := naga.Parse(BuildWGSL())
	if e != nil {
		t.Fatal(e)
	}
	ir, e := naga.LowerWithSource(ast, BuildWGSL())
	if e != nil {
		t.Fatal(e)
	}
	errs, e := naga.Validate(ir)
	if e != nil || len(errs) != 0 {
		t.Fatalf("%v %v", e, errs)
	}
}

// Package shader provides the single compute module used by both backends.
package shader

import (
	"encoding/binary"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"math"
)

// Layout (little endian), exactly 256 bytes, represented as 16 vec4<u32> in WGSL:
// 0: eye.xyz, fov (float32); 16: target.xyz, time (float32)
// 32: width, height, frame, flags (u32; flags bits 0..4 overlays, bit 5 normalize)
// 48: domain, height scale, CPU peak (float32), component count (u32)
// 64: 8 tightly packed 24-byte components: weight, mu_x, mu_y (float32),
// sigma_x/sigma_y (two IEEE binary16 values), rho (float32), color (0xRRGGBB).
// Packing sigmas is necessary to fit all requested data into 256 bytes. Both
// backends receive identical bytes, so their quantization is identical.
const UniformSize = 256
const ComponentOffset = 64
const ComponentStride = 24

func half(x float32) uint16 {
	b := math.Float32bits(x)
	e := int((b>>23)&255) - 127 + 15
	m := (b & 0x7fffff) + 0x1000
	if m&0x800000 != 0 {
		m = 0
		e++
	}
	return uint16((e << 10) | int(m>>13))
}
func Encode(s mixture.Scene, stats mixture.Stats, w, h int, t float32, frame uint32) []byte {
	b := make([]byte, UniformSize)
	put := func(o int, v uint32) { binary.LittleEndian.PutUint32(b[o:], v) }
	f := func(o int, v float64) { put(o, math.Float32bits(float32(v))) }
	c := s.Camera
	r := c.Radius * math.Cos(c.Pitch)
	f(0, c.Target[0]+r*math.Cos(c.Yaw))
	f(4, c.Target[1]+r*math.Sin(c.Yaw))
	f(8, c.Target[2]+c.Radius*math.Sin(c.Pitch))
	f(12, 45*math.Pi/180)
	for i, v := range c.Target {
		f(16+4*i, v)
	}
	f(28, float64(t))
	put(32, uint32(w))
	put(36, uint32(h))
	put(40, frame)
	flags := s.Overlays
	if s.Normalize {
		flags |= 32
	}
	put(44, flags)
	f(48, s.Domain)
	f(52, s.Height)
	f(56, stats.Peak[2])
	put(60, uint32(len(s.Components)))
	for i, c := range s.Components {
		o := 64 + i*24
		f(o, c.Weight/s.WeightSum())
		f(o+4, c.MuX)
		f(o+8, c.MuY)
		put(o+12, uint32(half(float32(c.SigmaX)))|uint32(half(float32(c.SigmaY)))<<16)
		f(o+16, c.Rho)
		put(o+20, c.Color)
	}
	return b
}
func Gaussian2(c mixture.Component, x, y float64) float64 { return mixture.Gaussian2(c, x, y) }

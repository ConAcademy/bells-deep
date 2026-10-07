// Package gpu serializes all native GPU activity on one locked executor thread.
package gpu

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"github.com/NimbleMarkets/bells-deep/internal/shader"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"
	"image"
	"math"
	"runtime"
	"sync"
	"time"
)

type Metrics struct {
	FPS         float64 `json:"fps"`
	RaymarchMS  float64 `json:"raymarch_ms"`
	ReadbackMS  float64 `json:"readback_ms"`
	UploadBytes int     `json:"upload_bytes"`
	Transport   string  `json:"transport"`
}
type job struct {
	fn   func(*device) error
	done chan error
}
type Executor struct {
	mu      sync.Mutex
	jobs    chan job
	closed  bool
	initErr error
}
type pipeline struct {
	module *wgpu.ShaderModule
	layout *wgpu.PipelineLayout
	bgl    *wgpu.BindGroupLayout
	pipe   *wgpu.ComputePipeline
}
type device struct {
	dev                   *wgpu.Device
	cache                 map[[32]byte]*pipeline
	uni, samples, staging *wgpu.Buffer
	texture               *wgpu.Texture
	view                  *wgpu.TextureView
	w, h, stride          int
	sampleHash            [32]byte
	sampleLen             int
}

func New() *Executor {
	e := &Executor{jobs: make(chan job)}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		d := &device{cache: map[[32]byte]*pipeline{}}
		inst, err := wgpu.CreateInstance(nil)
		if err == nil {
			defer inst.Release()
			var a *wgpu.Adapter
			a, err = inst.RequestAdapter(nil)
			if err == nil && a != nil {
				defer a.Release()
				info := a.Info()
				if info.DeviceType == gputypes.DeviceTypeCPU || info.Backend == gputypes.BackendEmpty {
					err = errors.New("no hardware adapter")
				} else {
					d.dev, err = a.RequestDevice(nil)
				}
			} else if err == nil {
				err = errors.New("no adapter")
			}
		}
		e.initErr = err
		close(ready)
		defer d.close()
		for j := range e.jobs {
			if e.initErr != nil {
				j.done <- e.initErr
			} else {
				j.done <- j.fn(d)
			}
		}
	}()
	<-ready
	return e
}
func (e *Executor) Available() error { return e.initErr }
func (e *Executor) do(fn func(*device) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("GPU executor closed")
	}
	j := job{fn, make(chan error, 1)}
	e.jobs <- j
	return <-j.done
}
func (e *Executor) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.closed = true
		done := make(chan error, 1)
		e.jobs <- job{func(d *device) error { d.close(); return nil }, done}
		<-done
		close(e.jobs)
	}
}
func (d *device) targetsClose() {
	if d.view != nil {
		d.view.Release()
		d.view = nil
	}
	if d.texture != nil {
		d.texture.Release()
		d.texture = nil
	}
	if d.staging != nil {
		d.staging.Release()
		d.staging = nil
	}
}
func (d *device) close() {
	d.targetsClose()
	if d.uni != nil {
		d.uni.Release()
		d.uni = nil
	}
	if d.samples != nil {
		d.samples.Release()
		d.samples = nil
	}
	for k, p := range d.cache {
		p.pipe.Release()
		p.layout.Release()
		p.bgl.Release()
		p.module.Release()
		delete(d.cache, k)
	}
	if d.dev != nil {
		d.dev.Release()
		d.dev = nil
	}
}
func (d *device) compile(code string) (*pipeline, error) {
	hash := sha256.Sum256([]byte(code))
	if p := d.cache[hash]; p != nil {
		return p, nil
	}
	m, err := d.dev.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "bells-deep", WGSL: code})
	if err != nil {
		return nil, err
	}
	bgl, err := d.dev.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}}, {Binding: 1, Visibility: wgpu.ShaderStageCompute, StorageTexture: &gputypes.StorageTextureBindingLayout{Access: gputypes.StorageTextureAccessWriteOnly, Format: gputypes.TextureFormatRGBA8Unorm, ViewDimension: gputypes.TextureViewDimension2D}}, {Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}}}})
	if err != nil {
		m.Release()
		return nil, err
	}
	layout, err := d.dev.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		bgl.Release()
		m.Release()
		return nil, err
	}
	pipe, err := d.dev.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Layout: layout, Module: m, EntryPoint: "main"})
	if err != nil {
		layout.Release()
		bgl.Release()
		m.Release()
		return nil, err
	}
	p := &pipeline{m, layout, bgl, pipe}
	d.cache[hash] = p
	return p, nil
}
func SampleBytes(points [][4]float32) []byte {
	b := make([]byte, max(16, len(points)*16))
	for i, p := range points {
		for j, x := range p {
			binary.LittleEndian.PutUint32(b[i*16+j*4:], math.Float32bits(x))
		}
	}
	if len(points) == 0 {
		binary.LittleEndian.PutUint32(b, math.Float32bits(1e9))
	}
	return b
}
func (e *Executor) Render(uniform, samples []byte, w, h int) (img *image.NRGBA, metrics Metrics, err error) {
	if w < 1 || h < 1 || w > 4096 || h > 4096 || len(uniform) != 256 || len(samples) < 16 || len(samples)%16 != 0 {
		return nil, metrics, errors.New("invalid render request")
	}
	err = e.do(func(d *device) error {
		p, er := d.compile(shader.BuildWGSL())
		if er != nil {
			return er
		}
		q := d.dev.Queue()
		if d.uni == nil {
			d.uni, er = d.dev.CreateBuffer(&wgpu.BufferDescriptor{Size: 256, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst})
			if er != nil {
				return er
			}
		}
		hash := sha256.Sum256(samples)
		if d.samples == nil || d.sampleHash != hash {
			if d.samples != nil {
				d.samples.Release()
				d.samples = nil
			}
			d.samples, er = d.dev.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(len(samples)), Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst})
			if er != nil {
				return er
			}
			if er = q.WriteBuffer(d.samples, 0, samples); er != nil {
				return er
			}
			d.sampleHash = hash
			d.sampleLen = len(samples)
		}
		if w != d.w || h != d.h || d.texture == nil {
			d.targetsClose()
			d.w, d.h = w, h
			d.stride = (w*4 + 255) &^ 255
			d.texture, er = d.dev.CreateTexture(&wgpu.TextureDescriptor{Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatRGBA8Unorm, Usage: gputypes.TextureUsageStorageBinding | gputypes.TextureUsageCopySrc})
			if er != nil {
				return er
			}
			d.view, er = d.dev.CreateTextureView(d.texture, nil)
			if er != nil {
				return er
			}
			d.staging, er = d.dev.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(d.stride * h), Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst})
			if er != nil {
				return er
			}
		}
		if er = q.WriteBuffer(d.uni, 0, uniform); er != nil {
			return er
		}
		bg, er := d.dev.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: p.bgl, Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: d.uni, Size: 256}, {Binding: 1, TextureView: d.view}, {Binding: 2, Buffer: d.samples, Size: uint64(d.sampleLen)}}})
		if er != nil {
			return er
		}
		defer bg.Release()
		start := time.Now()
		enc, er := d.dev.CreateCommandEncoder(nil)
		if er != nil {
			return er
		}
		pass, er := enc.BeginComputePass(nil)
		if er != nil {
			return er
		}
		pass.SetPipeline(p.pipe)
		pass.SetBindGroup(0, bg, nil)
		pass.Dispatch(uint32((w+7)/8), uint32((h+7)/8), 1)
		if er = pass.End(); er != nil {
			return er
		}
		enc.CopyTextureToBuffer(d.texture, d.staging, []wgpu.BufferTextureCopy{{BufferLayout: wgpu.ImageDataLayout{BytesPerRow: uint32(d.stride), RowsPerImage: uint32(h)}, TextureBase: wgpu.ImageCopyTexture{Texture: d.texture}, Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}}})
		cmd, er := enc.Finish()
		if er != nil {
			return er
		}
		defer cmd.Release()
		if _, er = q.Submit(cmd); er != nil {
			return er
		}
		pending, er := d.staging.MapAsync(wgpu.MapModeRead, 0, uint64(d.stride*h))
		if er != nil {
			return er
		}
		d.dev.Poll(wgpu.PollWait)
		er = pending.Wait(context.Background())
		pending.Release()
		metrics.RaymarchMS = float64(time.Since(start).Microseconds()) / 1000
		if er != nil {
			return er
		}
		defer d.staging.Unmap()
		start = time.Now()
		mapped, er := d.staging.MappedRange(0, uint64(d.stride*h))
		if er != nil {
			return er
		}
		defer mapped.Release()
		img = image.NewNRGBA(image.Rect(0, 0, w, h))
		b := mapped.Bytes()
		for y := 0; y < h; y++ {
			copy(img.Pix[y*img.Stride:y*img.Stride+w*4], b[y*d.stride:y*d.stride+w*4])
		}
		metrics.ReadbackMS = float64(time.Since(start).Microseconds()) / 1000
		return nil
	})
	return
}

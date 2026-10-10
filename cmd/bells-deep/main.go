// Command bells-deep explores bivariate Gaussian mixtures in a terminal or browser.
package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/NimbleMarkets/bells-deep/internal/gpu"
	"github.com/NimbleMarkets/bells-deep/internal/mixture"
	"github.com/NimbleMarkets/bells-deep/internal/preset"
	"github.com/NimbleMarkets/bells-deep/internal/session"
	webui "github.com/NimbleMarkets/bells-deep/internal/web"
	booba "github.com/NimbleMarkets/go-booba"
	"github.com/spf13/pflag"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bells-deep:", err)
		os.Exit(1)
	}
	holdRuntime()
}
func run() error {
	f := pflag.NewFlagSet("bells-deep", pflag.ContinueOnError)
	path := f.String("preset", "", "builtin name or .bell.json path")
	dir := f.String("gallery-dir", "bells-gallery", "saved scenes directory")
	fps := f.Int("fps", 60, "target frames per second (1..60)")
	addr := f.String("web", "", "serve browser UI on loopback")
	f.Lookup("web").NoOptDefVal = "127.0.0.1:8080"
	only := f.Bool("web-only", false, "browser only; no native GPU device")
	full := f.Bool("fullscreen", false, "viewport-only layout")
	width := f.Int("width", 1280, "soft raster width budget")
	height := f.Int("height", 720, "soft raster height budget")
	noGPU := f.Bool("no-gpu", false, "CPU analytic thumbnail preview")
	transport := f.String("kitty-transport", "auto", "Kitty transport: auto (local shared memory), direct, or shm; use direct over SSH/containers")
	format := f.String("kitty-format", "png", "direct Kitty format: png or rgba")
	normalize := f.String("normalize", "", "peaks or none")
	if err := f.Parse(os.Args[1:]); err != nil {
		if err == pflag.ErrHelp {
			return nil
		}
		return err
	}
	if *fps < 1 || *fps > 60 {
		return fmt.Errorf("--fps must be 1..60")
	}
	if *width < 1 || *height < 1 || *width > 4096 || *height > 4096 {
		return fmt.Errorf("--width and --height must be 1..4096")
	}
	s := mixture.Default()
	if *path != "" {
		var err error
		s, err = preset.Builtin(*path)
		if err != nil {
			s, err = preset.Load(*path)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "preset:", err, "— using isotropic")
			s = mixture.Default()
		}
	}
	switch *normalize {
	case "":
	case "peaks":
		s.Normalize = true
	case "none":
		s.Normalize = false
	default:
		return fmt.Errorf("--normalize must be peaks or none")
	}
	store := session.New(s, *dir)
	var server *webui.Server
	var err error
	if *only && *addr == "" {
		*addr = "127.0.0.1:8080"
	}
	if *addr != "" {
		server, err = webui.Start(*addr, store, *fps, *width, *height)
		if err != nil {
			return err
		}
		defer server.Close()
		fmt.Fprintln(os.Stderr, "bells-deep browser:", server.URL)
	}
	if *only {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)
		<-stop
		return nil
	}
	var exec *gpu.Executor
	if !*noGPU {
		exec = gpu.New()
		defer exec.Close()
		if err = exec.Available(); err != nil {
			fmt.Fprintln(os.Stderr, "GPU unavailable:", err, "— using CPU analytic preview")
			exec = nil
		}
	}
	m := newModel(store, exec, *fps, *width, *height)
	if err = configurePictures(m, *transport, *format); err != nil {
		return err
	}
	defer m.clearPictures() // Release pending/current shared-memory objects on every exit.
	m.fullscreen = *full
	m.server = server
	// booba.Run dispatches to native Bubble Tea or the WASM bridge by build target.
	if runtime.GOOS == "js" {
		// A browser has no POSIX signals, so the signal handler is useless there.
		return booba.Run(m, tea.WithoutSignalHandler())
	}
	return booba.Run(m)
}

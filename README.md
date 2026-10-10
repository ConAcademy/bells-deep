# bells-deep

[![Build Go](https://github.com/ConAcademy/bells-deep/actions/workflows/go.yml/badge.svg)](https://github.com/ConAcademy/bells-deep/actions/workflows/go.yml)
[![GitHub Pages](https://github.com/ConAcademy/bells-deep/actions/workflows/pages.yml/badge.svg)](https://conacademy.github.io/bells-deep/)

A keyboard-first explorer for 3D Gaussian bells. Adjust means, spreads,
correlation, and weights across up to eight components. See the height field,
covariance ellipses, analytic marginals, seeded samples, and peak contours.

Requires Go 1.26.8+ and a Metal, Vulkan, or other supported WebGPU adapter.
Terminal presentation uses NTCharts’ `picture` widget, including Kitty capability
probing, measured cell sizes, resize handling, tmux passthrough, and glyph fallback. `--no-gpu` provides a CPU analytic density preview for SSH.

```sh
task build
task run
task run -- --web-only --preset correlated
task run -- --no-gpu
task test
task race
task ci           # formatting, module, test, vet, and native/WASM build checks
task build-site   # browser build into site/dist (go-booba)
task serve-site   # serve site/dist at http://localhost:8765
```

[**Try it in your browser**](https://conacademy.github.io/bells-deep/): the TUI
compiled to WebAssembly and embedded with
[`go-booba`](https://github.com/NimbleMarkets/go-booba) v0.7.0, rendering
through the browser's WebGPU and Kitty shared memory. `go.mod` replaces Bubble
Tea and the clipboard package with forks that add js/wasm stubs; native builds
are unaffected. `task build-site` compiles `site/dist/app.wasm` and copies the
`site/` sources and the bundled booba terminal assets (`go tool booba-assets`)
into `site/dist/`. CI runs `task ci` on Linux and macOS for every push and pull
request. The Pages workflow builds the site on pull requests and pushes to
`main`; only `main` deploys. In repository **Settings → Pages**, choose
**GitHub Actions** as the build source once.

Without Task: `go run ./cmd/bells-deep`. Linux builds use
`CGO_ENABLED=0 go build -tags=nofakecgo -o bin/bells-deep ./cmd/bells-deep`.

| Key | Action |
| --- | --- |
| ↑ / ↓; ← / → | Select control; adjust (Shift = ten steps) |
| [ / ]; n / x | Select component; add / delete |
| r / R | Randomize component / mixture |
| Drag or h j k l; + / − | Orbit; zoom |
| Space; 0; Home | Pause; reset camera; fit every 3σ box |
| 1–5 | Grid, ellipses, marginals, samples, contours |
| f / Ctrl+F; Esc | Viewport only; restore |
| F3; F1 / ?; q | Gallery; help; quit |

Type `/` for commands, including `/set rho 0.8`, `/preset correlated`,
`/component 2`, `/add`, `/delete`, `/randomize`, `/pause`, `/play`, `/reset`,
`/save name`, `/load file.bell.json`, `/gallery`, `/fullscreen`, and `/quit`.
Scenes save atomically to `bells-gallery` (override with `--gallery-dir`).
Builtins: isotropic, correlated, ridge, offset, mixture, thin.

For each component, with `a=(x−μx)/σx`, `b=(y−μy)/σy`:

```text
f(x,y) = exp(−(a² + b² − 2ρab)/(2(1−ρ²))) / (2πσxσy√(1−ρ²))
height = Σ normalized_weight[i] × f[i](x,y)
```

`--normalize peaks` (default) divides by the numerically refined peak before
applying height scale; `--normalize none` keeps density units. The CPU HUD
always reports unscaled density, eigenvalues and 1σ semiaxes, condition number,
and weight sums. The ridge warning begins at |ρ|=0.97; the separate strict
singularity indicator means distance to |ρ|=1 is at most 1e−3 (outside the
interactive range). Sliders retain raw weights and the HUD shows normalized weights.

`--web` serves the embedded browser UI alongside the TUI; `--web-only` creates
no native device. Open the printed loopback URL (default 127.0.0.1:8080;
`--web=127.0.0.1:0` chooses a free port). The browser needs WebGPU. It dispatches
the exact native WGSL and presents a storage texture without pixel readback,
with at most two submissions in flight. Browser gallery access is read-only.
`--kitty-transport auto` uses shared memory for known local Kitty/Ghostty
terminals, with NTCharts’ direct fallback. Use `--kitty-transport direct` over
SSH or container boundaries; `--kitty-format rgba` selects uncompressed direct
frames instead of PNG. Gallery previews also use the picture widget.

No frontend build step or external network requests are used. GPU raster size
is bounded by `--width` / `--height` (1280×720), and `--fps` accepts 1–60.

The shared 256-byte uniform packs σx/σy as two binary16 values; other numeric
component parameters use float32. Both backends consume the same Go-encoded
bytes. CPU statistics retain float64 precision. `/api/state` exposes native
performance: raymarch_ms is host dispatch-to-map time (including GPU wait),
readback_ms is mapped-row copying, and upload_bytes is terminal payload size. Transport reports `png`, `rgba`,
`shm`, or `glyph` according to the active presentation path.

MIT · Copyright Neomantra Corp.

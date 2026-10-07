//go:build linux && !cgo && nofakecgo

package gpu

// nofakecgo disables goffi's runtime shim. Supply purego's implementation,
// including crosscall2, for the explicitly supported no-CGO Linux build.
import _ "github.com/ebitengine/purego"

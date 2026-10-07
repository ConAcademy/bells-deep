// Package assets embeds the browser files without a build step.
package assets

import "embed"

//go:embed index.html app.js app.css
var Files embed.FS

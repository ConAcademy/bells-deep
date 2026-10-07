package shader

import _ "embed"

//go:embed bell.wgsl
var source string

// BuildWGSL emits the same module for every preset and both backends.
func BuildWGSL() string { return source }

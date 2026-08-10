package webassets

import "embed"

// Assets holds the embedded production web assets.
//
//go:embed dist
var Assets embed.FS

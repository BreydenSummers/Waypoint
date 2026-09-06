// Package bundle embeds the checked-in export bundle tools so a deployed
// binary can build export bundles without the source tree on disk.
package bundle

import "embed"

//go:embed tools/verify-restore.mjs tools/regenerate-report.mjs
var Tools embed.FS

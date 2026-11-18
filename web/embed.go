// Package web embeds the built UI (npm run build writes dist/app).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// UI is the built app; empty (except .keep) when the UI was not built.
var UI, _ = fs.Sub(dist, "dist/app")

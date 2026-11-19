// Package templates holds the starter flake written into an empty repository.
package templates

import (
	"embed"
	"io/fs"
)

//go:embed all:fleet
var files embed.FS

// Fleet is a multi-machine flake: NixOS hosts, home-manager users and comin.
var Fleet, _ = fs.Sub(files, "fleet")

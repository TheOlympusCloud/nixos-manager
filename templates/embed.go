// Package templates holds the starter flake written into an empty repository.
package templates

import "embed"

// Fleet is a multi-machine flake: NixOS hosts, home-manager users and comin.
//
//go:embed all:fleet
var Fleet embed.FS

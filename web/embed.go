package web

import "embed"

//go:embed all:frontend/dist
var DistFS embed.FS

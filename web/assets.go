package web

import "embed"

// FS contains the embedded web console assets.
//
//go:embed *.html *.css *.js
var FS embed.FS

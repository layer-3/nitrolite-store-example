package web

import "embed"

// FS contains the embedded web console assets.
//
//go:embed index.html style.css console.js
var FS embed.FS

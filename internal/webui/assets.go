package webui

import "embed"

//go:embed dist
//go:embed dist/*
var embedded embed.FS

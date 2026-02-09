package web

import "embed"

// TemplateFS holds the embedded HTML template files.
//
//go:embed templates/*.html
var TemplateFS embed.FS

// StaticFS holds the embedded static assets (CSS, JS, images).
//
//go:embed static/*
var StaticFS embed.FS

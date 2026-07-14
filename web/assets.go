package webassets

import "embed"

// Dist contains the production Vite build embedded into lore-server.
//
//go:embed dist/index.html dist/assets/*
var Dist embed.FS

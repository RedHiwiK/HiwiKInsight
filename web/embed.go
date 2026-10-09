// Package web embeds the dashboard frontend build output (pnpm build writes to dist/app).
// dist/.gitkeep keeps the Go build working when the frontend is not built; the dashboard then reports that it is not built.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS

package webadmin

import "embed"

// FS holds the built admin dashboard. The dist directory is populated by the
// frontend build (pnpm build) before the Go binary is compiled; the committed
// placeholder keeps `go build` working on a fresh checkout.
//
//go:embed all:dist
var FS embed.FS

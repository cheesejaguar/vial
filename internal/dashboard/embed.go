// Package dashboard serves the vial web dashboard — a Svelte single-page
// application embedded directly into the binary.
//
// The Svelte SPA lives under web/ in the repository. Running "make dashboard"
// (or "cd web && npm run build") compiles it to static files and copies the
// output into internal/dashboard/static/. Those static files are then baked
// into the binary at compile time via the go:embed directive below.
//
// At runtime, [Server] serves the embedded files over HTTP, bound exclusively
// to 127.0.0.1, and exposes a JSON API under /api/ protected by a per-process
// Bearer token.
package dashboard

import "embed"

// The go:generate comment rebuilds the Svelte SPA and copies the output into
// the static/ directory that is embedded below. Run "go generate ./internal/dashboard/"
// (or the equivalent "make dashboard") before committing a new dashboard build.
//go:generate sh -c "cd ../../web && npm install && npm run build && rm -f ../internal/dashboard/static/index.html && cp -r build/* ../internal/dashboard/static/"

// frontendFS holds the compiled Svelte SPA static files embedded at compile
// time. The "all:" prefix ensures that files whose names start with "." or "_"
// (such as Vite's _app directory) are included, which is required for the
// SvelteKit build output structure.
//
// The subdirectory "static" is used as the root so that the embed path does
// not appear in served URLs; callers strip it with fs.Sub before serving.
//
//go:embed all:static
var frontendFS embed.FS

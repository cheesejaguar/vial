package dashboard

import (
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"testing/fstest"
)

// TestEmbeddedFSContainsJSFiles verifies that the embedded filesystem contains
// at least one compiled JavaScript file and that the MIME type resolved for the
// .js extension is not empty or text/html. The test is skipped when the
// dashboard has not been built yet (placeholder static/ directory).
func TestEmbeddedFSContainsJSFiles(t *testing.T) {
	staticFS, err := fs.Sub(frontendFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	// Walk and find JS files
	var jsFiles []string
	fs.WalkDir(staticFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(p, ".js") {
			jsFiles = append(jsFiles, p)
		}
		return nil
	})

	t.Logf("Found %d JS files in embedded FS", len(jsFiles))
	for _, f := range jsFiles {
		t.Logf("  %s", f)
	}

	if len(jsFiles) == 0 {
		t.Skip("No JS files in embedded FS — dashboard not built (run 'make dashboard' first)")
	}

	// Test that we can Open a JS file and get correct MIME type
	for _, jsFile := range jsFiles[:1] { // test first one
		f, err := staticFS.Open(jsFile)
		if err != nil {
			t.Errorf("cannot Open(%q): %v", jsFile, err)
			continue
		}
		f.Close()

		ext := path.Ext(jsFile)
		ct := mime.TypeByExtension(ext)
		t.Logf("MIME for %s: %q", ext, ct)
		if ct == "" || strings.Contains(ct, "html") {
			t.Errorf("MIME for .js = %q, want application/javascript", ct)
		}
	}
}

// TestServeJSFileWithCorrectMIME verifies that serveStaticFile sets
// Content-Type: application/javascript when serving a .js asset, and that the
// response body is the file's content. It uses an in-memory FS shaped like a
// SvelteKit build so it runs whether or not the dashboard has been built.
func TestServeJSFileWithCorrectMIME(t *testing.T) {
	staticFS := fstest.MapFS{
		"index.html":                    {Data: []byte("<!doctype html><html></html>")},
		"_app/immutable/entry/start.js": {Data: []byte("export const x = 1;")},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, staticFS)
	})

	req := httptest.NewRequest("GET", "/_app/immutable/entry/start.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want something with 'javascript'", ct)
	}
	if rec.Body.String() != "export const x = 1;" {
		t.Errorf("body = %q, want the JS file content", rec.Body.String())
	}
}

// TestServeUnknownPathFallsBackToIndex verifies the SPA fallback: a path with
// no matching file is answered with index.html so the client router can
// handle it.
func TestServeUnknownPathFallsBackToIndex(t *testing.T) {
	staticFS := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>spa</title>")},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, staticFS)
	})

	req := httptest.NewRequest("GET", "/secrets/OPENAI_API_KEY", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if rec.Body.String() != "<!doctype html><title>spa</title>" {
		t.Errorf("body = %q, want index.html content", rec.Body.String())
	}
}

// TestServeIndexHTML verifies that a GET / request returns the SPA entry point
// (index.html) with Content-Type: text/html.
func TestServeIndexHTML(t *testing.T) {
	staticFS, err := fs.Sub(frontendFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, staticFS)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	t.Logf("Request: / → Content-Type: %s (status %d, body len %d)", ct, rec.Code, rec.Body.Len())

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(ct, "html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

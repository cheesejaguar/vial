package dashboard

import (
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

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
		t.Fatal("No JS files found in embedded static FS — dashboard is placeholder only")
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

func TestServeJSFileWithCorrectMIME(t *testing.T) {
	staticFS, err := fs.Sub(frontendFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	// Find a JS file path
	var jsPath string
	fs.WalkDir(staticFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".js") && jsPath == "" {
			jsPath = p
		}
		return nil
	})

	if jsPath == "" {
		t.Skip("No JS files in embedded FS (placeholder build)")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, staticFS)
	})

	req := httptest.NewRequest("GET", "/"+jsPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	t.Logf("Request: /%s → Content-Type: %s (status %d)", jsPath, ct, rec.Code)

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want something with 'javascript'", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("empty response body")
	}
}

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

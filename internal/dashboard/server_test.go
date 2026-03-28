package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/awnumar/memguard"
	"github.com/charmbracelet/log"
	"os"

	"github.com/cheesejaguar/vial/internal/vault"
)

const testPort = 9876

// newTestServer creates a Server backed by an unlocked test vault with two
// secrets already stored: API_KEY ("secret-123") and DB_URL ("postgres://localhost").
func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")
	vm := vault.NewVaultManager(path)
	vm.SetKDFParams(vault.TestKDFParams())

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()
	if err := vm.Init(password); err != nil {
		t.Fatalf("Init vault: %v", err)
	}

	// Store test secrets
	for _, kv := range []struct{ k, v string }{
		{"API_KEY", "secret-123"},
		{"DB_URL", "postgres://localhost"},
	} {
		val := memguard.NewBufferFromBytes([]byte(kv.v))
		if err := vm.SetSecret(kv.k, val); err != nil {
			t.Fatalf("SetSecret %s: %v", kv.k, err)
		}
		val.Destroy()
	}

	logger := log.NewWithOptions(os.Stderr, log.Options{Level: log.FatalLevel})
	srv, err := NewServer(vm, nil, testPort, logger)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

// buildMux creates the same mux that Start() would build, but without
// starting a listener. It wraps the mux with corsHostMiddleware.
func buildMux(s *Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/lock", s.authMiddleware(s.handleLock))
	mux.HandleFunc("GET /api/auth/status", s.authMiddleware(s.handleAuthStatus))
	mux.HandleFunc("GET /api/vault/secrets", s.authMiddleware(s.handleListSecrets))
	mux.HandleFunc("GET /api/vault/secrets/{key}", s.authMiddleware(s.handleGetSecret))
	mux.HandleFunc("DELETE /api/vault/secrets/{key}", s.authMiddleware(s.handleDeleteSecret))
	mux.HandleFunc("POST /api/vault/secrets", s.authMiddleware(s.handleCreateSecret))
	mux.HandleFunc("GET /api/vault/secrets/{key}/reveal", s.authMiddleware(s.handleRevealSecret))
	mux.HandleFunc("GET /api/aliases", s.authMiddleware(s.handleListAliases))
	mux.HandleFunc("POST /api/aliases", s.authMiddleware(s.handleCreateAlias))
	mux.HandleFunc("DELETE /api/aliases/{alias}", s.authMiddleware(s.handleDeleteAlias))
	mux.HandleFunc("GET /api/projects", s.authMiddleware(s.handleListProjects))
	mux.HandleFunc("POST /api/projects", s.authMiddleware(s.handleCreateProject))
	mux.HandleFunc("DELETE /api/projects/{name}", s.authMiddleware(s.handleDeleteProject))
	mux.HandleFunc("GET /api/health/overview", s.authMiddleware(s.handleHealthOverview))
	mux.HandleFunc("GET /api/config", s.authMiddleware(s.handleGetConfig))
	return s.corsHostMiddleware(mux)
}

func doRequest(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", testPort)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return m
}

func decodeJSONArray(t *testing.T, rec *httptest.ResponseRecorder) []any {
	t.Helper()
	var a []any
	if err := json.NewDecoder(rec.Body).Decode(&a); err != nil {
		t.Fatalf("decode JSON array: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// Auth middleware tests
// ---------------------------------------------------------------------------

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/auth/status", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_WrongToken(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/auth/status", "bad-token")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_CorrectToken(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/auth/status", srv.Token())
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Host validation / CORS middleware tests
// ---------------------------------------------------------------------------

func TestHostValidation_WrongHost(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	req.Host = "evil.com"
	req.Header.Set("Authorization", "Bearer "+srv.Token())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for wrong host, got %d", rec.Code)
	}
}

func TestHostValidation_CorrectHost(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/auth/status", srv.Token())
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for correct host, got %d", rec.Code)
	}

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	expected := fmt.Sprintf("http://127.0.0.1:%d", testPort)
	if origin != expected {
		t.Errorf("CORS origin = %q, want %q", origin, expected)
	}
}

func TestHostValidation_Localhost(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	req.Host = fmt.Sprintf("localhost:%d", testPort)
	req.Header.Set("Authorization", "Bearer "+srv.Token())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for localhost, got %d", rec.Code)
	}
}

func TestCORS_OptionsRequest(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	req := httptest.NewRequest("OPTIONS", "/api/vault/secrets", nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", testPort)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing Access-Control-Allow-Methods header")
	}
}

// ---------------------------------------------------------------------------
// Auth status endpoint
// ---------------------------------------------------------------------------

func TestAuthStatus_Unlocked(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/auth/status", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	m := decodeJSON(t, rec)
	if locked, ok := m["locked"].(bool); !ok || locked {
		t.Errorf("expected locked=false, got %v", m["locked"])
	}
}

func TestAuthStatus_Locked(t *testing.T) {
	srv := newTestServer(t)
	srv.vm.Lock()
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/auth/status", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	m := decodeJSON(t, rec)
	if locked, ok := m["locked"].(bool); !ok || !locked {
		t.Errorf("expected locked=true, got %v", m["locked"])
	}
}

// ---------------------------------------------------------------------------
// List secrets
// ---------------------------------------------------------------------------

func TestListSecrets_ReturnsCorrectKeys(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/vault/secrets", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	arr := decodeJSONArray(t, rec)
	if len(arr) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(arr))
	}

	keys := map[string]bool{}
	for _, item := range arr {
		m := item.(map[string]any)
		keys[m["key"].(string)] = true
	}
	for _, k := range []string{"API_KEY", "DB_URL"} {
		if !keys[k] {
			t.Errorf("expected key %q in list", k)
		}
	}
}

func TestListSecrets_DoesNotExposeValues(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/vault/secrets", srv.Token())
	arr := decodeJSONArray(t, rec)

	for _, item := range arr {
		m := item.(map[string]any)
		if _, hasValue := m["value"]; hasValue {
			t.Errorf("list secrets should not include value, found for key %v", m["key"])
		}
	}
}

// ---------------------------------------------------------------------------
// Get secret (with optional reveal)
// ---------------------------------------------------------------------------

func TestGetSecret_WithoutReveal(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/vault/secrets/API_KEY", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	m := decodeJSON(t, rec)
	if m["key"] != "API_KEY" {
		t.Errorf("key = %v, want API_KEY", m["key"])
	}
	if _, hasValue := m["value"]; hasValue {
		t.Error("expected no value field without ?reveal=true")
	}
}

func TestGetSecret_WithReveal(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/vault/secrets/API_KEY?reveal=true", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	m := decodeJSON(t, rec)
	if m["value"] != "secret-123" {
		t.Errorf("value = %v, want secret-123", m["value"])
	}
}

func TestGetSecret_NotFound(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/vault/secrets/NONEXISTENT", srv.Token())
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Delete secret
// ---------------------------------------------------------------------------

func TestDeleteSecret_Success(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "DELETE", "/api/vault/secrets/API_KEY", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	m := decodeJSON(t, rec)
	if m["status"] != "deleted" {
		t.Errorf("status = %v, want deleted", m["status"])
	}

	// Verify secret is gone
	rec = doRequest(handler, "GET", "/api/vault/secrets/API_KEY", srv.Token())
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", rec.Code)
	}
}

func TestDeleteSecret_NotFound(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "DELETE", "/api/vault/secrets/NONEXISTENT", srv.Token())
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Vault locked -- endpoints return 403
// ---------------------------------------------------------------------------

func TestVaultLocked_EndpointsReturn403(t *testing.T) {
	srv := newTestServer(t)
	srv.vm.Lock()
	handler := buildMux(srv)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/vault/secrets"},
		{"GET", "/api/vault/secrets/API_KEY"},
		{"DELETE", "/api/vault/secrets/API_KEY"},
		{"GET", "/api/aliases"},
		{"GET", "/api/health/overview"},
	}

	for _, ep := range endpoints {
		rec := doRequest(handler, ep.method, ep.path, srv.Token())
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 when locked, got %d", ep.method, ep.path, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Aliases endpoint
// ---------------------------------------------------------------------------

func TestListAliases_Empty(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/aliases", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	arr := decodeJSONArray(t, rec)
	if len(arr) != 0 {
		t.Errorf("expected 0 aliases, got %d", len(arr))
	}
}

// ---------------------------------------------------------------------------
// Projects endpoint (nil registry)
// ---------------------------------------------------------------------------

func TestListProjects_NilRegistry(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/projects", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	arr := decodeJSONArray(t, rec)
	if len(arr) != 0 {
		t.Errorf("expected empty projects, got %d", len(arr))
	}
}

// ---------------------------------------------------------------------------
// Health overview
// ---------------------------------------------------------------------------

func TestHealthOverview(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	rec := doRequest(handler, "GET", "/api/health/overview", srv.Token())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	m := decodeJSON(t, rec)
	totalSecrets := int(m["total_secrets"].(float64))
	if totalSecrets != 2 {
		t.Errorf("total_secrets = %d, want 2", totalSecrets)
	}
	totalProjects := int(m["total_projects"].(float64))
	if totalProjects != 0 {
		t.Errorf("total_projects = %d, want 0", totalProjects)
	}
	secrets, ok := m["secrets"].([]any)
	if !ok {
		t.Fatal("expected secrets array in health overview")
	}
	if len(secrets) != 2 {
		t.Errorf("secrets array length = %d, want 2", len(secrets))
	}
}

// ---------------------------------------------------------------------------
// Lock endpoint
// ---------------------------------------------------------------------------

func TestLockEndpoint(t *testing.T) {
	srv := newTestServer(t)
	handler := buildMux(srv)

	// Vault starts unlocked
	if !srv.vm.IsUnlocked() {
		t.Fatal("vault should start unlocked")
	}

	req := httptest.NewRequest("POST", "/api/auth/lock", nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", testPort)
	req.Header.Set("Authorization", "Bearer "+srv.Token())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	m := decodeJSON(t, rec)
	if m["status"] != "locked" {
		t.Errorf("status = %v, want locked", m["status"])
	}

	if srv.vm.IsUnlocked() {
		t.Error("vault should be locked after POST /api/auth/lock")
	}
}

package dashboard

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"mime"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/awnumar/memguard"
	"github.com/charmbracelet/log"

	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/config"
	"github.com/cheesejaguar/vial/internal/project"
	"github.com/cheesejaguar/vial/internal/vault"
)

// Server is the dashboard HTTP server.
//
// It serves the embedded Svelte SPA for all non-API paths, and exposes a JSON
// API under /api/ for the dashboard frontend. All API routes require a Bearer
// token that is generated fresh on each Server construction and passed to the
// browser exclusively via the URL fragment (never logged or stored server-side).
//
// Security invariants:
//   - The listener is bound to 127.0.0.1 only; the OS prevents remote hosts
//     from connecting.
//   - Every request passes through corsHostMiddleware, which rejects Host
//     headers that do not match the expected 127.0.0.1:<port> or
//     localhost:<port> values, mitigating DNS rebinding attacks.
//   - The session token is compared with crypto/subtle.ConstantTimeCompare to
//     prevent timing-oracle attacks that could leak the token byte-by-byte.
//   - Secret values are never written to logs; only key names appear there.
type Server struct {
	vm       *vault.VaultManager  // vault the dashboard operates on
	registry *project.Registry    // project registry; may be nil if not configured
	auditLog *audit.Log           // structured audit trail written alongside the vault
	cfg      *config.Config       // application config; may be nil if WithConfig is not used
	token    string               // 32-byte random hex session token
	logger   *log.Logger          // structured logger for server lifecycle events
	port     int                  // TCP port the listener binds to on 127.0.0.1
}

// NewServer creates a new dashboard server.
//
// A cryptographically random 32-byte session token is generated during
// construction. The token is retrieved via Token() and included in the URL
// that is opened in the browser; it is never written to logs or persisted to
// disk. Optional ServerOptions are applied after the struct is initialised.
//
// An audit log is opened at the same directory as the vault file (audit.jsonl).
func NewServer(vm *vault.VaultManager, registry *project.Registry, port int, logger *log.Logger, opts ...ServerOption) (*Server, error) {
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating session token: %w", err)
	}

	// Co-locate the audit log with the vault file so both files are managed
	// under the same directory (default: ~/.local/share/vial/).
	auditPath := filepath.Join(filepath.Dir(vm.Path()), "audit.jsonl")
	auditLog := audit.NewLog(auditPath)

	s := &Server{
		vm:       vm,
		registry: registry,
		auditLog: auditLog,
		token:    token,
		logger:   logger,
		port:     port,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// ServerOption is a functional option for configuring optional Server fields
// that are not required for basic operation.
type ServerOption func(*Server)

// WithConfig attaches application config to the server, making it available
// via the /api/config endpoint. Without this option, /api/config returns an
// empty object.
func WithConfig(cfg *config.Config) ServerOption {
	return func(s *Server) {
		s.cfg = cfg
	}
}

// Token returns the session token for the dashboard.
//
// The token is a 64-character lowercase hex string derived from 32 random
// bytes. It is intended to be embedded in the dashboard URL as a fragment
// (e.g. http://127.0.0.1:9876/#token=<token>) so that it is never sent to
// the server in HTTP logs or in the Referer header when following external
// links.
func (s *Server) Token() string {
	return s.token
}

// Start binds the HTTP listener and begins serving requests. It blocks until
// the server encounters an unrecoverable error.
//
// The listener is bound exclusively to 127.0.0.1 to prevent remote access.
// All requests pass through corsHostMiddleware before reaching any handler.
// API routes are additionally wrapped by authMiddleware.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes — all protected by authMiddleware (Bearer token check).
	mux.HandleFunc("POST /api/auth/lock", s.authMiddleware(s.handleLock))
	mux.HandleFunc("GET /api/auth/status", s.authMiddleware(s.handleAuthStatus))
	mux.HandleFunc("GET /api/vault/secrets", s.authMiddleware(s.handleListSecrets))
	mux.HandleFunc("GET /api/vault/secrets/{key}", s.authMiddleware(s.handleGetSecret))
	mux.HandleFunc("DELETE /api/vault/secrets/{key}", s.authMiddleware(s.handleDeleteSecret))
	mux.HandleFunc("GET /api/aliases", s.authMiddleware(s.handleListAliases))
	mux.HandleFunc("GET /api/projects", s.authMiddleware(s.handleListProjects))
	mux.HandleFunc("POST /api/vault/secrets", s.authMiddleware(s.handleCreateSecret))
	mux.HandleFunc("GET /api/vault/secrets/{key}/reveal", s.authMiddleware(s.handleRevealSecret))
	mux.HandleFunc("GET /api/health/overview", s.authMiddleware(s.handleHealthOverview))
	mux.HandleFunc("GET /api/audit", s.authMiddleware(s.handleAudit))
	mux.HandleFunc("POST /api/projects", s.authMiddleware(s.handleCreateProject))
	mux.HandleFunc("DELETE /api/projects/{name}", s.authMiddleware(s.handleDeleteProject))
	mux.HandleFunc("POST /api/aliases", s.authMiddleware(s.handleCreateAlias))
	mux.HandleFunc("DELETE /api/aliases/{alias}", s.authMiddleware(s.handleDeleteAlias))
	mux.HandleFunc("GET /api/config", s.authMiddleware(s.handleGetConfig))

	// Serve the embedded Svelte SPA for all non-API paths. The "static"
	// subdirectory is stripped via fs.Sub so URL paths map directly to
	// filenames without a "static/" prefix.
	staticFS, err := fs.Sub(frontendFS, "static")
	if err != nil {
		return fmt.Errorf("accessing embedded frontend: %w", err)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, staticFS)
	})

	// Bind exclusively to the loopback address. Using 127.0.0.1 rather than
	// ":port" ensures the OS rejects connections from external interfaces even
	// if the firewall is misconfigured.
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	// Wrap the mux with the Host-validation and CORS middleware. This is the
	// outermost layer; every request passes through it before reaching any
	// route handler.
	handler := s.corsHostMiddleware(mux)

	s.logger.Info("Dashboard running", "url", fmt.Sprintf("http://%s", addr))
	return http.Serve(listener, handler)
}

// corsHostMiddleware validates the Host header and sets CORS response headers.
//
// Host validation defends against DNS rebinding attacks: a malicious webpage
// on an attacker-controlled domain could otherwise instruct the browser to
// make requests to 127.0.0.1:<port> and read the JSON API responses. By
// rejecting any Host value that is not exactly "127.0.0.1:<port>" or
// "localhost:<port>", the middleware ensures that only the browser tab that
// was intentionally opened by the user can reach the API.
//
// CORS headers allow the Svelte dev server (npm run dev, which proxies to the
// Go backend) to function during development without loosening the production
// security model. The allowed origin is pinned to the exact 127.0.0.1 origin
// so that unrelated pages on localhost cannot cross-origin read API responses.
func (s *Server) corsHostMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		portStr := fmt.Sprintf("%d", s.port)
		allowedHosts := []string{
			"127.0.0.1:" + portStr,
			"localhost:" + portStr,
		}

		host := r.Host
		hostValid := false
		for _, h := range allowedHosts {
			if host == h {
				hostValid = true
				break
			}
		}
		if !hostValid {
			// Reject immediately without calling any downstream handler so
			// that no partial processing occurs on a potentially spoofed
			// request.
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Pin CORS to the 127.0.0.1 origin only. The Vary header tells
		// intermediate caches that the response differs per Origin value.
		origin := fmt.Sprintf("http://127.0.0.1:%d", s.port)
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")

		// Handle CORS preflight. Preflights do not carry an auth token so
		// they must be answered before authMiddleware runs.
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			// Cache preflight results for 24 hours to reduce preflight traffic.
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware validates the session token on every API request.
//
// The token must be present in the Authorization header as a Bearer credential.
// Comparison uses crypto/subtle.ConstantTimeCompare to prevent timing-oracle
// attacks: a naive string comparison would return early on the first mismatched
// byte, allowing an attacker to probe the token one byte at a time by
// measuring response latency.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		// ConstantTimeCompare returns 1 only when both slices are equal in
		// length and content, evaluated in constant time regardless of where
		// the first difference occurs.
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// handleAuthStatus reports whether the vault is currently locked.
// The dashboard polls this endpoint to update the lock indicator in the UI.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"locked": !s.vm.IsUnlocked(),
	})
}

// handleLock locks the vault and destroys the in-memory DEK.
// After this call, all secret-access endpoints will return 403 until the vault
// is unlocked again (which currently requires restarting the dashboard).
func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	s.vm.Lock()
	writeJSON(w, map[string]any{"status": "locked"})
}

// handleListSecrets returns metadata for all secrets in the vault.
// Secret values are intentionally excluded from this listing; use
// handleRevealSecret (/api/vault/secrets/{key}/reveal) to fetch a specific
// value when needed.
func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	secrets := s.vm.ListSecrets()
	result := make([]map[string]any, 0, len(secrets))
	for _, sec := range secrets {
		result = append(result, map[string]any{
			"key":     sec.Key,
			"aliases": sec.Metadata.Aliases,
			"tags":    sec.Metadata.Tags,
			"added":   sec.Metadata.Added.Format(time.RFC3339),
			"rotated": sec.Metadata.Rotated.Format(time.RFC3339),
		})
	}
	writeJSON(w, result)
}

// handleGetSecret returns metadata for a single secret identified by its key.
//
// By default the encrypted value is not included. Appending "?reveal=true" to
// the query string decrypts and includes the plaintext value in the response.
// The reveal parameter is an explicit opt-in so that the dashboard only
// transmits secret values when the user actively requests them.
func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	key := r.PathValue("key")
	meta, err := s.vm.GetMetadata(key)
	if err != nil {
		http.Error(w, "secret not found", http.StatusNotFound)
		return
	}

	result := map[string]any{
		"key":     key,
		"aliases": meta.Aliases,
		"tags":    meta.Tags,
		"added":   meta.Added.Format(time.RFC3339),
		"rotated": meta.Rotated.Format(time.RFC3339),
	}

	// Optionally reveal the decrypted value when the caller explicitly opts in.
	// The LockedBuffer is destroyed immediately after the value is serialised
	// to avoid holding plaintext in the heap longer than necessary.
	if r.URL.Query().Get("reveal") == "true" {
		val, err := s.vm.GetSecret(key)
		if err == nil {
			result["value"] = string(val.Bytes())
			val.Destroy()
		}
	}

	writeJSON(w, result)
}

// handleDeleteSecret removes a secret from the vault by key.
func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	key := r.PathValue("key")
	if err := s.vm.RemoveSecret(key); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"status": "deleted", "key": key})
}

// handleListAliases returns a flat list of all alias → canonical key mappings
// across every secret in the vault. Aliases are derived from secret metadata,
// not stored in a separate index; this handler denormalises them for the UI.
func (s *Server) handleListAliases(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	secrets := s.vm.ListSecrets()
	var aliases []map[string]string
	for _, sec := range secrets {
		for _, a := range sec.Metadata.Aliases {
			aliases = append(aliases, map[string]string{
				"alias":     a,
				"canonical": sec.Key,
			})
		}
	}
	// Return an empty JSON array rather than null when there are no aliases,
	// so the frontend can always iterate without a nil check.
	if aliases == nil {
		aliases = []map[string]string{}
	}
	writeJSON(w, aliases)
}

// handleListProjects returns the list of registered projects from the project
// registry. For each project, it also reports which .env files were found on
// disk so the dashboard can display them without a separate round-trip.
// Returns an empty array when no registry is configured.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		writeJSON(w, []any{})
		return
	}

	projects := s.registry.List()
	result := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		envFiles := project.FindEnvFiles(p.Path)
		entry := map[string]any{
			"name":      p.Name,
			"path":      p.Path,
			"added_at":  p.AddedAt.Format(time.RFC3339),
			"env_files": envFiles,
		}
		if p.LastPoured != nil {
			entry["last_poured"] = p.LastPoured.Format(time.RFC3339)
		}
		result = append(result, entry)
	}
	writeJSON(w, result)
}

// handleHealthOverview returns aggregate health metrics for the vault.
//
// "Stale" secrets are those whose Added timestamp is more than 90 days old.
// "Overdue" secrets are those that have a configured rotation schedule and
// whose last rotation is past that interval. The per-secret breakdown is
// included so the dashboard can highlight individual problematic keys.
func (s *Server) handleHealthOverview(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	secrets := s.vm.ListSecrets()
	now := time.Now()
	staleCount := 0
	var secretHealth []map[string]any

	overdueCount := 0
	for _, sec := range secrets {
		ageDays := int(math.Floor(now.Sub(sec.Metadata.Added).Hours() / 24))
		rotatedDays := int(math.Floor(now.Sub(sec.Metadata.Rotated).Hours() / 24))
		// Secrets older than 90 days without rotation are considered stale.
		if ageDays > 90 {
			staleCount++
		}

		entry := map[string]any{
			"key":      sec.Key,
			"age_days": ageDays,
		}
		if !sec.Metadata.Rotated.IsZero() {
			entry["last_rotated"] = sec.Metadata.Rotated.Format(time.RFC3339)
			entry["rotated_days_ago"] = rotatedDays
		}
		if sec.Metadata.RotationDays > 0 {
			entry["rotation_days"] = sec.Metadata.RotationDays
			overdue := rotatedDays > sec.Metadata.RotationDays
			entry["rotation_overdue"] = overdue
			if overdue {
				overdueCount++
			}
		}
		secretHealth = append(secretHealth, entry)
	}

	projectCount := 0
	if s.registry != nil {
		projectCount = len(s.registry.List())
	}

	writeJSON(w, map[string]any{
		"total_secrets":  len(secrets),
		"total_projects": projectCount,
		"stale_count":    staleCount,
		"overdue_count":  overdueCount,
		"secrets":        secretHealth,
	})
}

// handleAudit returns the 100 most recent audit log entries in reverse
// chronological order. The audit log is a newline-delimited JSON file
// (audit.jsonl) co-located with the vault file.
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.auditLog.Read(100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		result = append(result, map[string]any{
			"timestamp": e.Timestamp.Format(time.RFC3339),
			"event":     e.Event,
			"keys":      e.Keys,
			"project":   e.Project,
			"detail":    e.Detail,
		})
	}
	writeJSON(w, result)
}

// handleCreateSecret creates a new secret in the vault.
//
// The request body must be a JSON object with "key" and "value" fields.
// The value is wrapped in a memguard.LockedBuffer for the duration of the
// SetSecret call and immediately destroyed afterwards, minimising the window
// during which the plaintext is held in process memory.
func (s *Server) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	// Wrap the value in a LockedBuffer so it occupies mlock'd memory for the
	// duration of the vault write. Destroy is called on all code paths.
	buf := memguard.NewBufferFromBytes([]byte(req.Value))
	if err := s.vm.SetSecret(req.Key, buf); err != nil {
		buf.Destroy()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	buf.Destroy()

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"status": "created", "key": req.Key})
}

// handleRevealSecret decrypts and returns the plaintext value of a single
// secret. The LockedBuffer is destroyed via defer immediately after the value
// is serialised into the response, so the plaintext does not persist on the
// heap after the handler returns.
func (s *Server) handleRevealSecret(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	key := r.PathValue("key")
	val, err := s.vm.GetSecret(key)
	if err != nil {
		http.Error(w, "secret not found", http.StatusNotFound)
		return
	}
	// Destroy the buffer as soon as the handler returns; the JSON encoder
	// will have already read the bytes by then.
	defer val.Destroy()

	writeJSON(w, map[string]any{"key": key, "value": string(val.Bytes())})
}

// handleCreateProject registers a new project directory in the project
// registry. The request body must be a JSON object with a "path" field
// containing the absolute path to the project directory.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		http.Error(w, "project registry not configured", http.StatusInternalServerError)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	proj, err := s.registry.Add(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"name":     proj.Name,
		"path":     proj.Path,
		"added_at": proj.AddedAt.Format(time.RFC3339),
	})
}

// handleDeleteProject removes a project from the registry by its display name.
// This does not touch the project directory or any .env files.
func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		http.Error(w, "project registry not configured", http.StatusInternalServerError)
		return
	}

	name := r.PathValue("name")
	if err := s.registry.Remove(name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"status": "removed", "name": name})
}

// handleCreateAlias adds an alias to the metadata of an existing canonical
// secret. If the alias already exists on that secret the request is treated as
// a no-op and 201 is returned, making the operation idempotent.
func (s *Server) handleCreateAlias(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	var req struct {
		Alias     string `json:"alias"`
		Canonical string `json:"canonical"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Alias == "" || req.Canonical == "" {
		http.Error(w, "alias and canonical are required", http.StatusBadRequest)
		return
	}

	meta, err := s.vm.GetMetadata(req.Canonical)
	if err != nil {
		http.Error(w, "canonical key not found", http.StatusNotFound)
		return
	}

	// Idempotent: if the alias already exists, return success without
	// modifying metadata or triggering a vault write.
	for _, a := range meta.Aliases {
		if a == req.Alias {
			writeJSON(w, map[string]any{"status": "created"})
			return
		}
	}

	meta.Aliases = append(meta.Aliases, req.Alias)
	if err := s.vm.SetMetadata(req.Canonical, *meta); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"status": "created"})
}

// handleDeleteAlias removes an alias from whichever secret currently holds it.
// The search is performed by scanning all secrets because aliases are stored
// inline in secret metadata rather than in a separate index.
func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request) {
	if !s.vm.IsUnlocked() {
		http.Error(w, "vault is locked", http.StatusForbidden)
		return
	}

	alias := r.PathValue("alias")
	secrets := s.vm.ListSecrets()

	for _, sec := range secrets {
		for i, a := range sec.Metadata.Aliases {
			if a == alias {
				// Splice out the alias at index i while preserving order.
				meta := sec.Metadata
				meta.Aliases = append(meta.Aliases[:i], meta.Aliases[i+1:]...)
				if err := s.vm.SetMetadata(sec.Key, meta); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, map[string]any{"status": "removed"})
				return
			}
		}
	}

	http.Error(w, "alias not found", http.StatusNotFound)
}

// handleGetConfig returns a subset of the application configuration that is
// safe to expose to the dashboard frontend. Returns an empty object when no
// config was attached via WithConfig.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSON(w, map[string]any{})
		return
	}
	writeJSON(w, map[string]any{
		"vault_path":      s.cfg.VaultPath,
		"session_timeout": s.cfg.SessionTimeout.String(),
		"env_example":     s.cfg.EnvExample,
		"log_level":       s.cfg.LogLevel,
	})
}

// serveStaticFile serves a file from the embedded filesystem with the correct
// Content-Type header derived from the file extension.
//
// SPA routing fallback: if the requested path does not correspond to an
// embedded file, index.html is served instead. This allows the Svelte router
// to handle client-side navigation paths such as /projects or /health without
// the server needing to know about them.
//
// Directory paths fall through to an inner index.html lookup before falling
// back to the top-level index.html.
//
// Cache-Control: assets under /immutable/ are fingerprinted by the build tool
// and can be cached indefinitely; all other files are served without cache
// headers to ensure the latest build is always loaded.
func serveStaticFile(w http.ResponseWriter, r *http.Request, staticFS fs.FS) {
	urlPath := r.URL.Path
	if urlPath == "/" {
		urlPath = "/index.html"
	}

	// fs.FS uses slash-separated relative paths; strip the leading slash that
	// HTTP request paths always carry.
	filePath := strings.TrimPrefix(urlPath, "/")

	f, err := staticFS.Open(filePath)
	if err != nil {
		// File not found: serve index.html so the SPA router can handle the
		// path on the client side.
		filePath = "index.html"
		f, err = staticFS.Open(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}

	// Resolve directory entries to their inner index.html.
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		f.Close()
		filePath = filePath + "/index.html"
		f, err = staticFS.Open(filePath)
		if err != nil {
			// Fall back to the top-level SPA entry point.
			f, _ = staticFS.Open("index.html")
			if f == nil {
				http.NotFound(w, r)
				return
			}
			filePath = "index.html"
		}
		stat, _ = f.Stat()
	}
	// Defer close after all potential file handle reassignments above to
	// avoid closing a handle that has already been replaced.
	defer f.Close()

	// Derive Content-Type from the extension. mime.TypeByExtension consults
	// the OS MIME database and Go's built-in type table. Fall back to
	// application/octet-stream for unknown extensions to avoid browsers
	// misinterpreting files as text/html.
	ext := path.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	// SvelteKit places content-hashed bundles under _app/immutable/. These
	// filenames change whenever the content changes, so they can be cached
	// indefinitely without the risk of serving a stale file.
	if strings.Contains(filePath, "/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	if stat != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}

	io.Copy(w, f)
}

// writeJSON sets the Content-Type header and encodes data as JSON into w.
// Encoding errors are silently dropped because the status code has typically
// already been written at this point and there is no clean way to signal
// a mid-response encoding failure to the client.
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// generateToken creates a cryptographically random 32-byte session token
// encoded as a 64-character lowercase hex string.
//
// The token is generated once per Server construction and is used as the
// Bearer credential for all API requests. It is never persisted to disk or
// written to logs; it is only passed to the browser via the URL fragment.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// URL returns the full dashboard URL with the session token embedded in the
// fragment.
//
// The fragment (#token=...) is chosen deliberately: URL fragments are not sent
// to the server in HTTP requests, are not recorded in server logs, and are not
// included in the Referer header when the user navigates away from the page.
// This means the token can appear safely in the URL bar without being captured
// by network logging infrastructure.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/#token=%s", s.port, s.token)
}

// DefaultPort returns the default TCP port for the dashboard server.
func DefaultPort() int {
	return 9876
}

// RegistryPath returns the filesystem path to the projects.json registry file,
// derived from the vault file path. The registry is stored alongside the vault
// so that both files are under the same user-data directory.
func RegistryPath(vaultPath string) string {
	return filepath.Join(filepath.Dir(vaultPath), "projects.json")
}

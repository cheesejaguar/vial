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

	"github.com/charmbracelet/log"

	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/project"
	"github.com/cheesejaguar/vial/internal/vault"
)

// Server is the dashboard HTTP server.
type Server struct {
	vm       *vault.VaultManager
	registry *project.Registry
	auditLog *audit.Log
	token    string
	logger   *log.Logger
	port     int
}

// NewServer creates a new dashboard server.
func NewServer(vm *vault.VaultManager, registry *project.Registry, port int, logger *log.Logger) (*Server, error) {
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating session token: %w", err)
	}

	auditPath := filepath.Join(filepath.Dir(vm.Path()), "audit.jsonl")
	auditLog := audit.NewLog(auditPath)

	return &Server{
		vm:       vm,
		registry: registry,
		auditLog: auditLog,
		token:    token,
		logger:   logger,
		port:     port,
	}, nil
}

// Token returns the session token for the dashboard.
func (s *Server) Token() string {
	return s.token
}

// Start starts the HTTP server on localhost.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("POST /api/auth/lock", s.authMiddleware(s.handleLock))
	mux.HandleFunc("GET /api/auth/status", s.authMiddleware(s.handleAuthStatus))
	mux.HandleFunc("GET /api/vault/secrets", s.authMiddleware(s.handleListSecrets))
	mux.HandleFunc("GET /api/vault/secrets/{key}", s.authMiddleware(s.handleGetSecret))
	mux.HandleFunc("DELETE /api/vault/secrets/{key}", s.authMiddleware(s.handleDeleteSecret))
	mux.HandleFunc("GET /api/aliases", s.authMiddleware(s.handleListAliases))
	mux.HandleFunc("GET /api/projects", s.authMiddleware(s.handleListProjects))
	mux.HandleFunc("GET /api/health/overview", s.authMiddleware(s.handleHealthOverview))
	mux.HandleFunc("GET /api/audit", s.authMiddleware(s.handleAudit))

	// Serve embedded SPA
	staticFS, err := fs.Sub(frontendFS, "static")
	if err != nil {
		return fmt.Errorf("accessing embedded frontend: %w", err)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, staticFS)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	handler := s.corsHostMiddleware(mux)

	s.logger.Info("Dashboard running", "url", fmt.Sprintf("http://%s", addr))
	return http.Serve(listener, handler)
}

// corsHostMiddleware validates the Host header and sets CORS headers to
// protect against DNS rebinding and cross-origin attacks.
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
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		origin := fmt.Sprintf("http://127.0.0.1:%d", s.port)
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware validates the session token.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"locked": !s.vm.IsUnlocked(),
	})
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	s.vm.Lock()
	writeJSON(w, map[string]any{"status": "locked"})
}

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

	// Optionally reveal the value
	if r.URL.Query().Get("reveal") == "true" {
		val, err := s.vm.GetSecret(key)
		if err == nil {
			result["value"] = string(val.Bytes())
			val.Destroy()
		}
	}

	writeJSON(w, result)
}

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
	if aliases == nil {
		aliases = []map[string]string{}
	}
	writeJSON(w, aliases)
}

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

// serveStaticFile serves a file from the embedded FS with correct MIME types.
// Falls back to index.html for SPA client-side routing.
func serveStaticFile(w http.ResponseWriter, r *http.Request, staticFS fs.FS) {
	urlPath := r.URL.Path
	if urlPath == "/" {
		urlPath = "/index.html"
	}

	// Strip leading slash for fs.Open
	filePath := strings.TrimPrefix(urlPath, "/")

	f, err := staticFS.Open(filePath)
	if err != nil {
		// SPA fallback: serve index.html for client-side routes
		filePath = "index.html"
		f, err = staticFS.Open(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	defer f.Close()

	// Get file info for size
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		// If it's a directory, try index.html inside it
		f.Close()
		filePath = filePath + "/index.html"
		f, err = staticFS.Open(filePath)
		if err != nil {
			// SPA fallback
			f, _ = staticFS.Open("index.html")
			if f == nil {
				http.NotFound(w, r)
				return
			}
			filePath = "index.html"
		}
		defer f.Close()
		stat, _ = f.Stat()
	}

	// Set Content-Type based on file extension
	ext := path.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	// Cache immutable assets aggressively
	if strings.Contains(filePath, "/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	if stat != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}

	io.Copy(w, f)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// URL returns the dashboard URL with the session token in the fragment.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/#token=%s", s.port, s.token)
}

// DefaultPort returns the default dashboard port.
func DefaultPort() int {
	return 9876
}

// RegistryPath returns the projects.json path for the registry.
func RegistryPath(vaultPath string) string {
	return filepath.Join(filepath.Dir(vaultPath), "projects.json")
}

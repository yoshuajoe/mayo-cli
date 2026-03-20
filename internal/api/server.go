package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mayo-cli/internal/ai"
	"mayo-cli/internal/config"
	"mayo-cli/internal/db"
	"mayo-cli/internal/session"
	"mayo-cli/internal/ui"
)

// Server holds all dependencies for the Master Mayo HTTP API server.
type Server struct {
	Registry map[string]*ai.Orchestrator
	Token    string
	Port     int
	Config   *config.Config
	mu       sync.RWMutex
}

// NewServer creates a new API server instance.
func NewServer(cfg *config.Config, token string, port int) *Server {
	return &Server{
		Registry: make(map[string]*ai.Orchestrator),
		Token:    token,
		Port:     port,
		Config:   cfg,
	}
}

// Start initializes routes and starts listening.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Multi-session routes: /v1/:session_id/query
	mux.Handle("/v1/", s.authMiddleware(http.HandlerFunc(s.router)))
	mux.Handle("/v1/sessions", s.authMiddleware(http.HandlerFunc(s.handleListSessions)))

	addr := ":8080"
	ui.PrintSuccess(fmt.Sprintf("Mayo Master API Server listening on http://localhost%s", addr))
	ui.PrintInfo("Endpoints: /v1/:session_id/query, /v1/sessions")
	if s.Token != "" {
		ui.PrintInfo("Auth: Bearer token security ACTIVE.")
	}

	return http.ListenAndServe(addr, corsMiddleware(mux))
}

func (s *Server) router(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found, expected /v1/:session_id/:action"})
		return
	}

	sessionID := parts[1]
	action := parts[2]

	switch action {
	case "query":
		s.handleQuery(w, r, sessionID)
	case "status":
		s.handleStatus(w, r, sessionID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action: " + action})
	}
}

// authMiddleware validates the Bearer token on every request.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Token != "" {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") || strings.TrimPrefix(authHeader, "Bearer ") != s.Token {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers so browser apps can call the API.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// QueryRequest is the JSON body for POST /v1/:session_id/query.
type QueryRequest struct {
	Query string `json:"query"`
}

// getOrchestrator loads a session into memory only when needed (Lazy Loading)
func (s *Server) getOrchestrator(sessionID string) (*ai.Orchestrator, error) {
	// 0. Security Whitelist: Only allow sessions that have been "spawned" via CLI
	if !s.isSessionAuthorized(sessionID) {
		return nil, fmt.Errorf("session [%s] is not authorized for this API port. Run '/serve spawn' in CLI to authorize it", sessionID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Check if already loaded
	if orch, exists := s.Registry[sessionID]; exists {
		return orch, nil
	}

	// 2. Load session metadata from disk
	sess, err := session.LoadSessionMetadata(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found")
	}

	if s.Config == nil {
		return nil, fmt.Errorf("server config not loaded")
	}

	// 3. Initialize AI Profile for this session
	var activeProf *config.AIProfile
	for i, p := range s.Config.AIProfiles {
		if p.Name == s.Config.ActiveAIProfile {
			activeProf = &s.Config.AIProfiles[i]
			break
		}
	}
	if activeProf == nil {
		return nil, fmt.Errorf("no active AI profile found")
	}


	apiKey, err := activeProf.GetAPIKey(s.Config.UseKeyring)
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("AI Profile incomplete (API Key missing or keyring error: %v)", err)
	}
	aiClient := ai.NewClient(activeProf.Provider, apiKey, activeProf.DefaultModel)
	if aiClient == nil {
		return nil, fmt.Errorf("AI initialization failed for provider: %s", activeProf.Provider)
	}

	// 4. Create Orchestrator
	orch := &ai.Orchestrator{
		AI:             aiClient,
		Connections:    make(map[string]*ai.DBConnection),
		Session:        sess,
		DefaultLimit:   s.Config.DefaultLimit,
		AnalystEnabled: s.Config.AnalystEnabled,
		Interactive:    false, // No confirmation in API mode
	}

	// 5. Headless Re-Connection: Load database sources saved in session
	for _, alias := range sess.ConnectedProfiles {
		for _, ds := range s.Config.DSProfiles {
			if ds.Name == alias {
				conn, err := db.Connect(ds.Driver, ds.DSN)
				if err == nil {
					orch.Connections[alias] = &ai.DBConnection{
						Alias:  alias,
						Driver: ds.Driver,
						DB:     conn,
					}
					// Sync schema metadata
					orch.LoadMetadata(alias)
				}
				break
			}
		}
	}

	// 6. Initialize privacy vault for this session
	session.InitVault(sess)

	s.Registry[sessionID] = orch
	return orch, nil
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	list, _ := session.ListSessions()
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	orch, err := s.getOrchestrator(sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	resp, err := orch.ProcessQuery(ctx, req.Query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"response":   resp,
		"session_id": sessionID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, sessionID string) {
	orch, err := s.getOrchestrator(sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	model := ""
	if orch.AI != nil {
		model = fmt.Sprintf("%s/%s", orch.AI.GetProvider(), orch.AI.GetModel())
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":            "ready",
		"session_id":        sessionID,
		"connected_sources": len(orch.Connections),
		"ai_model":          model,
	})
}

func (s *Server) isSessionAuthorized(sessionID string) bool {
	dir := filepath.Join(config.GetConfigDir(), "api")
	path := filepath.Join(dir, "servers.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var servers []RunningServer
	json.Unmarshal(data, &servers)

	for _, rs := range servers {
		if rs.Port == s.Port && rs.SessionID == sessionID {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

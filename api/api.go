// Package api implements the "Go HTTP API 与嵌入式前端" component: the JSON
// write endpoints, health check, task detail, adapter/audit diagnostic routes,
// and static frontend embedding. Every write endpoint delegates to the
// transactional service layer and surfaces stable error codes and sorted
// reasons on rejection (failure boundary #2).
package api

import (
	"encoding/json"
	"net/http"

	"leafwash-packaging-release-gate/store"
)

// Server bundles the service layer with the HTTP routing surface.
type Server struct {
	store store.Store
	mux   *http.ServeMux
}

// New constructs a Server with all documented routes registered.
func New(s store.Store) *Server {
	srv := &Server{store: s, mux: http.NewServeMux()}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	s.mux.HandleFunc("POST /api/tasks/lock", s.handleLock)
	s.mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("POST /api/tasks/{id}/feed-confirmations", s.handleFeedConfirm)
	s.mux.HandleFunc("POST /api/tasks/{id}/curve-samples", s.handleCurveSample)
	s.mux.HandleFunc("POST /api/tasks/{id}/atp-swabs", s.handleATPSwab)
	s.mux.HandleFunc("POST /api/tasks/{id}/microbiology-readings", s.handleMicrobiology)
	s.mux.HandleFunc("POST /api/tasks/{id}/rechecks", s.handleRecheck)
	s.mux.HandleFunc("POST /api/tasks/{id}/reviews", s.handleReview)
	s.mux.HandleFunc("POST /api/tasks/{id}/finalize", s.handleFinalize)
	s.mux.HandleFunc("GET /api/tasks/{id}/adapter-calls", s.handleAdapterCalls)
	s.mux.HandleFunc("GET /api/tasks/{id}/audit", s.handleAudit)
	// The root serves the embedded frontend (registered by cmd via WithFrontend).
}

// WithFrontend registers the embedded frontend file system at the root route.
func (s *Server) WithFrontend(fs http.FileSystem) *Server {
	s.mux.Handle("/", http.FileServer(fs))
	return s
}

// Handler returns the composed HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Health(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.store.GetTask(r.Context(), id)
	if !ok {
		writeError(w, store.NewAppError(store.CodeNotFound, "task not found"))
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func errorBody(code string, reasons []string) map[string]any {
	return map[string]any{"error_code": code, "reasons": reasons}
}

// writeError maps an AppError to an HTTP status and writes the JSON body.
func writeError(w http.ResponseWriter, err error) {
	code := store.AppErrorCode(err)
	if code == "" {
		writeJSON(w, http.StatusInternalServerError, errorBody("INTERNAL", []string{err.Error()}))
		return
	}
	ae := err.(*store.AppError)
	writeJSON(w, statusForCode(code), errorBody(ae.Code, ae.Reasons))
}

// statusForCode maps a stable error code to an HTTP status.
func statusForCode(code string) int {
	switch code {
	case store.CodeNotFound, store.CodeUnknownBlindCode:
		return http.StatusNotFound
	case store.CodeIdempotencyConflict, store.CodeOccupied,
		store.CodeTerminalState, store.CodeFinalizeConflict,
		store.CodeRecheckAlreadyExists, store.CodeBlindCodeDuplicate,
		store.CodeReviewDuplicate, store.CodeReviewOverlap:
		return http.StatusConflict
	case store.CodeStaleRevision, store.CodeSealMismatch,
		store.CodeInvalidState, store.CodeGenerationMismatch,
		store.CodeInvalidReading, store.CodeDuplicateTime,
		store.CodeMissingTime, store.CodeCoverageIncomplete,
		store.CodePersonNotQualified, store.CodeBlindCodeRevealed,
		store.CodeArithmeticError, store.CodeAnomalyPresent,
		store.CodeValidationFailed:
		return http.StatusUnprocessableEntity
	default:
		if len(code) > 0 && code[0] == 'A' && len(code) >= 8 && code[:8] == "ADAPTER_" {
			return http.StatusServiceUnavailable
		}
		return http.StatusBadRequest
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

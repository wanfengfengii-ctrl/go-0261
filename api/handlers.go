package api

import (
	"encoding/json"
	"net/http"

	"leafwash-packaging-release-gate/store"
)

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	var req store.LockRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.LockTask(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleFeedConfirm(w http.ResponseWriter, r *http.Request) {
	var req store.FeedConfirmRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.FeedConfirm(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCurveSample(w http.ResponseWriter, r *http.Request) {
	var req store.CurveSampleRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.SubmitCurveSample(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleATPSwab(w http.ResponseWriter, r *http.Request) {
	var req store.ATPSwabRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.SubmitATPSwab(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMicrobiology(w http.ResponseWriter, r *http.Request) {
	var req store.MicrobiologyRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.SubmitMicrobiology(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRecheck(w http.ResponseWriter, r *http.Request) {
	var req store.RecheckRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.SubmitRecheck(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req store.ReviewRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.SubmitReview(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleFinalize(w http.ResponseWriter, r *http.Request) {
	var req store.FinalizeRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", []string{err.Error()}))
		return
	}
	resp, err := s.store.Finalize(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdapterCalls(w http.ResponseWriter, r *http.Request) {
	calls, err := s.store.ListAdapterCalls(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"adapter_calls": calls})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.ListAudit(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit_events": events})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

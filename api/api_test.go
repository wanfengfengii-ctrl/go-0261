package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/store"
)

func newBody(s string) io.Reader {
	return strings.NewReader(s)
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cat := catalog.NewMemory()
	catalog.Seed(cat)
	backend, err := store.OpenSQLite(filepath.Join(t.TempDir(), "api.db"), cat)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { backend.Close() })
	return New(store.NewService(backend))
}

func TestHealth(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}
}

func TestGetTask(t *testing.T) {
	s := newTestServer(t)
	lockReq := store.LockRequest{}
	lockReq.TaskID = "TASK-API"
	lockReq.BaseLotID = "BL-2026-001"
	lockReq.SealID = "SEAL-001"
	lockReq.PrecoolLot = "PC-001"
	lockReq.CutLineID = "CUT-3"
	lockReq.WashTankID = "TANK-A"
	lockReq.FormulaID = "F-100"
	lockReq.FormulaRevision = 3
	lockReq.SampleTimes = []int64{0, 300, 600}
	lockReq.BlindCodes = []string{"BLIND-01", "BLIND-02", "BLIND-03"}
	lockReq.ATPPoints = []string{"ATP-1", "ATP-2", "ATP-3"}
	lockReq.PlateWells = []string{"WELL-A1", "WELL-A2", "WELL-B1"}
	lockReq.DrainSlots = []string{"DRAIN-1"}
	lockReq.Reviewers = []string{"P-2001", "P-2002"}
	if _, err := s.store.LockTask(context.Background(), lockReq); err != nil {
		t.Fatalf("lock: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/TASK-API", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get task status = %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/tasks/MISSING", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing task status = %d, want 404", rec.Code)
	}
}

func TestLockEndpoint(t *testing.T) {
	s := newTestServer(t)
	body := `{"task_id":"TASK-LOCK","base_lot_id":"BL-2026-001","seal_id":"SEAL-001",
		"precool_lot":"PC-001","cut_line_id":"CUT-3","wash_tank_id":"TANK-A",
		"formula_id":"F-100","formula_revision":3,
		"sample_times":[0,300,600],"blind_codes":["BLIND-01","BLIND-02","BLIND-03"],
		"atp_points":["ATP-1","ATP-2","ATP-3"],
		"plate_wells":["WELL-A1","WELL-A2","WELL-B1"],
		"drain_slots":["DRAIN-1"],"reviewers":["P-2001","P-2002"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/lock", newBody(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("lock status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
}

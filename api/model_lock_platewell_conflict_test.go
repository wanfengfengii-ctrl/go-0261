package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/lease"
	"leafwash-packaging-release-gate/store"
	"leafwash-packaging-release-gate/task"
)

func TestModel_LockTaskRejectsPlateWellReuseAcrossOpenTasks(t *testing.T) {
	cat := catalog.NewMemory()
	catalog.Seed(cat)
	for _, well := range []string{"WELL-C1", "WELL-C2", "WELL-C3"} {
		cat.AddPlateWell(catalog.PlateWellTemplate{WellCode: well})
	}
	backend, err := store.OpenSQLite(filepath.Join(t.TempDir(), "api.db"), cat)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	srv := New(store.NewService(backend))

	lettuce := store.LockRequest{LockSpec: task.LockSpec{
		TaskID:          "TASK-A",
		BaseLotID:       "BL-2026-001",
		SealID:          "SEAL-001",
		PrecoolLot:      "PC-001",
		CutLineID:       "CUT-3",
		WashTankID:      "TANK-A",
		FormulaID:       "F-100",
		FormulaRevision: 3,
		SampleTimes:     []int64{0, 300, 600},
		BlindCodes:      []string{"BLIND-01", "BLIND-02", "BLIND-03"},
		ATPPoints:       []string{"ATP-1", "ATP-2", "ATP-3"},
		PlateWells:      []string{"WELL-A1", "WELL-A2", "WELL-B1"},
		DrainSlots:      []string{"DRAIN-1", "DRAIN-2"},
		Reviewers:       []string{"P-2001", "P-2002"},
	}}
	spinachConflict := store.LockRequest{LockSpec: task.LockSpec{
		TaskID:          "TASK-B",
		BaseLotID:       "BL-2026-002",
		SealID:          "SEAL-011",
		PrecoolLot:      "PC-011",
		CutLineID:       "CUT-3",
		WashTankID:      "TANK-B",
		FormulaID:       "F-100",
		FormulaRevision: 3,
		SampleTimes:     []int64{0, 300, 600},
		BlindCodes:      []string{"BLIND-11", "BLIND-12", "BLIND-13"},
		ATPPoints:       []string{"ATP-1", "ATP-2", "ATP-3"},
		PlateWells:      []string{"WELL-A1", "WELL-A2", "WELL-B1"},
		DrainSlots:      []string{"DRAIN-3", "DRAIN-4"},
		Reviewers:       []string{"P-2001", "P-2002"},
	}}
	spinachNoConflict := spinachConflict
	spinachNoConflict.TaskID = "TASK-C"
	spinachNoConflict.PlateWells = []string{"WELL-C1", "WELL-C2", "WELL-C3"}

	postLock := func(t *testing.T, req store.LockRequest) (int, []byte) {
		t.Helper()
		body, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal lock request: %v", err)
		}
		httpReq := httptest.NewRequest(http.MethodPost, "/api/tasks/lock", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httpReq)
		return rec.Code, rec.Body.Bytes()
	}
	requireFullLeaseSet := func(t *testing.T, body []byte, taskID string, resources []lease.Resource) {
		t.Helper()
		var resp store.LockResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode lock response: %v; body=%s", err, string(body))
		}
		if resp.TaskID != taskID || resp.Generation != 1 || resp.State != task.StatePendingFeed {
			t.Fatalf("unexpected lock identity/state: %+v", resp)
		}
		if len(resp.Leases) != len(resources) {
			t.Fatalf("lease count = %d, want %d; leases=%+v", len(resp.Leases), len(resources), resp.Leases)
		}
		want := map[lease.Resource]bool{}
		for _, r := range resources {
			want[r] = true
		}
		for _, got := range resp.Leases {
			r := lease.Resource{Type: got.ResourceType, Key: got.ResourceKey}
			if !want[r] {
				t.Fatalf("unexpected lease in response: %+v", r)
			}
			delete(want, r)
		}
		if len(want) != 0 {
			t.Fatalf("missing leases from response: %+v", want)
		}
	}
	get := func(t *testing.T, path string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes()
	}

	cases := []struct {
		name       string
		req        store.LockRequest
		wantStatus int
		wantCode   string
		wantLeases []lease.Resource
		after      func(t *testing.T)
	}{
		{
			name:       "first open task receives complete typed leases",
			req:        lettuce,
			wantStatus: http.StatusCreated,
			wantLeases: []lease.Resource{
				{Type: lease.ResourceBlindCode, Key: "BLIND-01"},
				{Type: lease.ResourceBlindCode, Key: "BLIND-02"},
				{Type: lease.ResourceBlindCode, Key: "BLIND-03"},
				{Type: lease.ResourceDrainSlot, Key: "DRAIN-1"},
				{Type: lease.ResourceDrainSlot, Key: "DRAIN-2"},
				{Type: lease.ResourcePlateWell, Key: "WELL-A1"},
				{Type: lease.ResourcePlateWell, Key: "WELL-A2"},
				{Type: lease.ResourcePlateWell, Key: "WELL-B1"},
				{Type: lease.ResourceWashTank, Key: "TANK-A"},
			},
		},
		{
			name:       "same task same generation can reenter existing leases",
			req:        lettuce,
			wantStatus: http.StatusCreated,
			wantLeases: []lease.Resource{
				{Type: lease.ResourceBlindCode, Key: "BLIND-01"},
				{Type: lease.ResourceBlindCode, Key: "BLIND-02"},
				{Type: lease.ResourceBlindCode, Key: "BLIND-03"},
				{Type: lease.ResourceDrainSlot, Key: "DRAIN-1"},
				{Type: lease.ResourceDrainSlot, Key: "DRAIN-2"},
				{Type: lease.ResourcePlateWell, Key: "WELL-A1"},
				{Type: lease.ResourcePlateWell, Key: "WELL-A2"},
				{Type: lease.ResourcePlateWell, Key: "WELL-B1"},
				{Type: lease.ResourceWashTank, Key: "TANK-A"},
			},
		},
		{
			name:       "different open task cannot reuse plate wells with fresh tank drain and blind codes",
			req:        spinachConflict,
			wantStatus: http.StatusConflict,
			wantCode:   store.CodeOccupied,
			after: func(t *testing.T) {
				status, body := get(t, "/api/tasks/TASK-B")
				if status != http.StatusNotFound {
					t.Fatalf("TASK-B status = %d, want 404 after rejected lock; body=%s", status, string(body))
				}
				status, body = get(t, "/api/tasks/TASK-B/audit")
				if status != http.StatusOK {
					t.Fatalf("TASK-B audit status = %d, want 200; body=%s", status, string(body))
				}
				var auditBody struct {
					AuditEvents []store.AuditEvent `json:"audit_events"`
				}
				if err := json.Unmarshal(body, &auditBody); err != nil {
					t.Fatalf("decode audit response: %v; body=%s", err, string(body))
				}
				if len(auditBody.AuditEvents) != 0 {
					t.Fatalf("TASK-B audit events = %d, want 0", len(auditBody.AuditEvents))
				}
			},
		},
		{
			name:       "fresh resources from rejected task remain available",
			req:        spinachNoConflict,
			wantStatus: http.StatusCreated,
			wantLeases: []lease.Resource{
				{Type: lease.ResourceBlindCode, Key: "BLIND-11"},
				{Type: lease.ResourceBlindCode, Key: "BLIND-12"},
				{Type: lease.ResourceBlindCode, Key: "BLIND-13"},
				{Type: lease.ResourceDrainSlot, Key: "DRAIN-3"},
				{Type: lease.ResourceDrainSlot, Key: "DRAIN-4"},
				{Type: lease.ResourcePlateWell, Key: "WELL-C1"},
				{Type: lease.ResourcePlateWell, Key: "WELL-C2"},
				{Type: lease.ResourcePlateWell, Key: "WELL-C3"},
				{Type: lease.ResourceWashTank, Key: "TANK-B"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := postLock(t, tc.req)
			if status != tc.wantStatus {
				t.Fatalf("lock status = %d, want %d; body=%s", status, tc.wantStatus, string(body))
			}
			if tc.wantCode != "" {
				var errBody struct {
					ErrorCode string `json:"error_code"`
				}
				if err := json.Unmarshal(body, &errBody); err != nil {
					t.Fatalf("decode error response: %v; body=%s", err, string(body))
				}
				if errBody.ErrorCode != tc.wantCode {
					t.Fatalf("error_code = %q, want %q; body=%s", errBody.ErrorCode, tc.wantCode, string(body))
				}
			}
			if tc.wantLeases != nil {
				requireFullLeaseSet(t, body, tc.req.TaskID, tc.wantLeases)
			}
			if tc.after != nil {
				tc.after(t)
			}
		})
	}
}

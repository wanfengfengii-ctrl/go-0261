package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"leafwash-packaging-release-gate/task"
)

func TestModel_DuplicateLockPreservesProgressedTask(t *testing.T) {
	const taskID = "TASK-DUP"

	tests := []struct {
		name                   string
		confirmers             []string
		wantState              task.State
		duplicateLockBody      string
		wantDuplicateRejection bool
	}{
		{
			name:              "identical retry after one feed confirmation keeps confirmer",
			confirmers:        []string{"P-1001"},
			wantState:         task.StatePendingFeed,
			duplicateLockBody: lockDuplicateModelBody(t, taskID, "SEAL-001", "PC-001"),
		},
		{
			name:                   "changed retry after feed advancement is rejected and leaves task untouched",
			confirmers:             []string{"P-1001", "P-1002"},
			wantState:              task.StateTanksOccupied,
			duplicateLockBody:      lockDuplicateModelBody(t, taskID, "SEAL-002", "PC-002"),
			wantDuplicateRejection: true,
		},
	}

	for caseIndex, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t)
			handler := server.Handler()
			originalLockBody := lockDuplicateModelBody(t, taskID, "SEAL-001", "PC-001")

			rec := postDuplicateModelJSON(t, handler, "/api/tasks/lock", originalLockBody)
			if rec.Code != http.StatusCreated {
				t.Fatalf("initial lock status = %d, want 201; body=%s", rec.Code, rec.Body.String())
			}

			for confirmerIndex, personID := range tc.confirmers {
				body := feedDuplicateModelBody(t, fmt.Sprintf("OP-%d-feed-%d", caseIndex, confirmerIndex), personID)
				rec = postDuplicateModelJSON(t, handler, "/api/tasks/"+taskID+"/feed-confirmations", body)
				if rec.Code != http.StatusOK {
					t.Fatalf("feed confirmation %s status = %d, want 200; body=%s", personID, rec.Code, rec.Body.String())
				}
			}

			before := getDuplicateModelTask(t, handler, taskID)
			if before.State != tc.wantState {
				t.Fatalf("state before duplicate lock = %q, want %q", before.State, tc.wantState)
			}
			if !reflect.DeepEqual(before.FeedConfirmers, tc.confirmers) {
				t.Fatalf("feed confirmers before duplicate lock = %v, want %v", before.FeedConfirmers, tc.confirmers)
			}
			beforeSnapshot, err := before.Snapshot()
			if err != nil {
				t.Fatalf("decode snapshot before duplicate lock: %v", err)
			}

			rec = postDuplicateModelJSON(t, handler, "/api/tasks/lock", tc.duplicateLockBody)
			if rec.Code >= http.StatusInternalServerError {
				t.Fatalf("duplicate lock status = %d, want stable rejection or no-op success; body=%s", rec.Code, rec.Body.String())
			}
			if tc.wantDuplicateRejection && rec.Code < http.StatusBadRequest {
				t.Fatalf("changed duplicate lock status = %d, want rejection; body=%s", rec.Code, rec.Body.String())
			}

			after := getDuplicateModelTask(t, handler, taskID)
			if after.State != before.State {
				t.Fatalf("state after duplicate lock = %q, want preserved %q", after.State, before.State)
			}
			if !reflect.DeepEqual(after.FeedConfirmers, before.FeedConfirmers) {
				t.Fatalf("feed confirmers after duplicate lock = %v, want preserved %v", after.FeedConfirmers, before.FeedConfirmers)
			}
			afterSnapshot, err := after.Snapshot()
			if err != nil {
				t.Fatalf("decode snapshot after duplicate lock: %v", err)
			}
			if !reflect.DeepEqual(afterSnapshot, beforeSnapshot) {
				t.Fatalf("snapshot after duplicate lock = %+v, want preserved %+v", afterSnapshot, beforeSnapshot)
			}
		})
	}
}

func lockDuplicateModelBody(t *testing.T, taskID, sealID, precoolLot string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"task_id":          taskID,
		"base_lot_id":      "BL-2026-001",
		"seal_id":          sealID,
		"precool_lot":      precoolLot,
		"cut_line_id":      "CUT-3",
		"wash_tank_id":     "TANK-A",
		"formula_id":       "F-100",
		"formula_revision": 3,
		"sample_times":     []int64{0, 300, 600},
		"blind_codes":      []string{"BLIND-01", "BLIND-02", "BLIND-03"},
		"atp_points":       []string{"ATP-1", "ATP-2", "ATP-3"},
		"plate_wells":      []string{"WELL-A1", "WELL-A2", "WELL-B1"},
		"drain_slots":      []string{"DRAIN-1"},
		"reviewers":        []string{"P-2001", "P-2002"},
	})
	if err != nil {
		t.Fatalf("marshal lock body: %v", err)
	}
	return string(body)
}

func feedDuplicateModelBody(t *testing.T, operationNo, personID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"operation_no": operationNo,
		"person_id":    personID,
		"base_lot_id":  "BL-2026-001",
		"seal_id":      "SEAL-001",
		"cut_line_id":  "CUT-3",
	})
	if err != nil {
		t.Fatalf("marshal feed body: %v", err)
	}
	return string(body)
}

func postDuplicateModelJSON(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func getDuplicateModelTask(t *testing.T, handler http.Handler, taskID string) task.InspectionTask {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+taskID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get task status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got task.InspectionTask
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	return got
}

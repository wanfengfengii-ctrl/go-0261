package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"leafwash-packaging-release-gate/store"
)

func TestModel_IdempotencyReplayRequiresRouteTaskAndOperationScope(t *testing.T) {
	type response struct {
		status int
		body   map[string]any
	}

	post := func(t *testing.T, s *Server, path, body string) response {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, newBody(body))
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)

		var decoded map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("decode %s response: %v; raw=%s", path, err, rec.Body.String())
		}
		return response{status: rec.Code, body: decoded}
	}

	get := func(t *testing.T, s *Server, path string) response {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)

		var decoded map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("decode %s response: %v; raw=%s", path, err, rec.Body.String())
		}
		return response{status: rec.Code, body: decoded}
	}

	lock := func(t *testing.T, s *Server, taskID, sealID, precoolLot, cutLineID, tankID, blindCode, drainSlot string) {
		t.Helper()
		body := fmt.Sprintf(`{
			"task_id":%q,
			"base_lot_id":"BL-2026-001",
			"seal_id":%q,
			"precool_lot":%q,
			"cut_line_id":%q,
			"wash_tank_id":%q,
			"formula_id":"F-100",
			"formula_revision":3,
			"sample_times":[0],
			"blind_codes":[%q],
			"atp_points":[],
			"plate_wells":[],
			"drain_slots":[%q],
			"reviewers":["P-2001","P-2002"]
		}`, taskID, sealID, precoolLot, cutLineID, tankID, blindCode, drainSlot)
		if got := post(t, s, "/api/tasks/lock", body); got.status != http.StatusCreated {
			t.Fatalf("lock %s status = %d, want 201; body=%v", taskID, got.status, got.body)
		}
	}

	feed := func(operationNo, personID string) string {
		return fmt.Sprintf(`{
			"operation_no":%q,
			"generation":1,
			"person_id":%q,
			"base_lot_id":"BL-2026-001",
			"seal_id":"SEAL-001",
			"cut_line_id":"CUT-3"
		}`, operationNo, personID)
	}

	conflict := func(t *testing.T, got response) {
		t.Helper()
		if got.status != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%v", got.status, got.body)
		}
		if got.body["error_code"] != store.CodeIdempotencyConflict {
			t.Fatalf("error_code = %v, want %s; body=%v", got.body["error_code"], store.CodeIdempotencyConflict, got.body)
		}
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "same route task kind generation and content replays the feed confirmation",
			run: func(t *testing.T) {
				s := newTestServer(t)
				lock(t, s, "TASK-A", "SEAL-001", "PC-001", "CUT-3", "TANK-A", "BLIND-A", "DRAIN-A")

				body := feed("OP-REPLAY", "P-1001")
				first := post(t, s, "/api/tasks/TASK-A/feed-confirmations", body)
				if first.status != http.StatusOK || first.body["task_id"] != "TASK-A" {
					t.Fatalf("first feed response = %d %v, want TASK-A success", first.status, first.body)
				}
				second := post(t, s, "/api/tasks/TASK-A/feed-confirmations", body)
				if second.status != http.StatusOK || second.body["task_id"] != first.body["task_id"] {
					t.Fatalf("replay response = %d %v, want original TASK-A response", second.status, second.body)
				}
				if fmt.Sprint(second.body["confirmed_by"]) != fmt.Sprint(first.body["confirmed_by"]) {
					t.Fatalf("replay confirmed_by = %v, want %v", second.body["confirmed_by"], first.body["confirmed_by"])
				}
			},
		},
		{
			name: "same operation number with different content is an idempotency conflict",
			run: func(t *testing.T) {
				s := newTestServer(t)
				lock(t, s, "TASK-A", "SEAL-001", "PC-001", "CUT-3", "TANK-A", "BLIND-A", "DRAIN-A")

				if got := post(t, s, "/api/tasks/TASK-A/feed-confirmations", feed("OP-CONFLICT", "P-1001")); got.status != http.StatusOK {
					t.Fatalf("first feed status = %d, want 200; body=%v", got.status, got.body)
				}
				conflict(t, post(t, s, "/api/tasks/TASK-A/feed-confirmations", feed("OP-CONFLICT", "P-1002")))
			},
		},
		{
			name: "same operation number and content on another route task does not replay the original task",
			run: func(t *testing.T) {
				s := newTestServer(t)
				lock(t, s, "TASK-A", "SEAL-001", "PC-001", "CUT-3", "TANK-A", "BLIND-A", "DRAIN-A")
				lock(t, s, "TASK-B", "SEAL-002", "PC-002", "CUT-9", "TANK-B", "BLIND-B", "DRAIN-B")

				body := feed("OP-CROSS-TASK", "P-1001")
				if got := post(t, s, "/api/tasks/TASK-A/feed-confirmations", body); got.status != http.StatusOK {
					t.Fatalf("TASK-A feed status = %d, want 200; body=%v", got.status, got.body)
				}
				got := post(t, s, "/api/tasks/TASK-B/feed-confirmations", body)
				if got.status == http.StatusOK && got.body["task_id"] == "TASK-A" {
					t.Fatalf("cross-task replay returned TASK-A response through TASK-B route: %v", got.body)
				}
				conflict(t, got)

				taskB := get(t, s, "/api/tasks/TASK-B")
				if taskB.status != http.StatusOK {
					t.Fatalf("get TASK-B status = %d, want 200; body=%v", taskB.status, taskB.body)
				}
				if gotConfirmers, ok := taskB.body["feed_confirmers"].([]any); ok && len(gotConfirmers) != 0 {
					t.Fatalf("TASK-B feed_confirmers = %v, want none", gotConfirmers)
				}
			},
		},
		{
			name: "same operation number on another interface is an idempotency conflict",
			run: func(t *testing.T) {
				s := newTestServer(t)
				lock(t, s, "TASK-A", "SEAL-001", "PC-001", "CUT-3", "TANK-A", "BLIND-A", "DRAIN-A")

				if got := post(t, s, "/api/tasks/TASK-A/feed-confirmations", feed("OP-CROSS-KIND", "P-1001")); got.status != http.StatusOK {
					t.Fatalf("first feed status = %d, want 200; body=%v", got.status, got.body)
				}
				if got := post(t, s, "/api/tasks/TASK-A/feed-confirmations", feed("OP-ADVANCE", "P-1002")); got.status != http.StatusOK {
					t.Fatalf("second feed status = %d, want 200; body=%v", got.status, got.body)
				}
				curveBody := `{
					"operation_no":"OP-CROSS-KIND",
					"generation":1,
					"sample_time":0,
					"chlorine_x100":"1.50",
					"orp_mv":700,
					"ph_x100":"7.00",
					"temperature_x100":"5.00",
					"turbidity_x100":"0.50"
				}`
				conflict(t, post(t, s, "/api/tasks/TASK-A/curve-samples", curveBody))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

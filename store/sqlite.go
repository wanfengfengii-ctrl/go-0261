package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/lease"
	"leafwash-packaging-release-gate/task"
)

// SQLite is the production Persistence implementation backed by a SQLite
// database in WAL mode. It recovers open tasks, leases, pending adapter calls,
// idempotency records, and current state after a process restart (failure
// boundary #7).
type SQLite struct {
	db  *sql.DB
	cat catalog.Catalog
}

// OpenSQLite opens (or creates) the SQLite database at path, applies the schema,
// and configures WAL journaling. The parent directory is created when needed.
func OpenSQLite(path string, cat catalog.Catalog) (*SQLite, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single writer connection serializes transactions and avoids SQLITE_BUSY
	// while still providing restart-recoverable durability through WAL.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable wal: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure pragmas: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &SQLite{db: db, cat: cat}, nil
}

// Catalog returns the read-side catalog used at lock time.
func (s *SQLite) Catalog() catalog.Catalog { return s.cat }

// Health reports whether the database is reachable.
func (s *SQLite) Health(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the underlying database handle.
func (s *SQLite) Close() error { return s.db.Close() }

// Tx runs fn within a single transaction. write=true uses an immediate
// write transaction so concurrent writers contend deterministically.
func (s *SQLite) Tx(ctx context.Context, write bool, fn func(tx Tx) error) error {
	var (
		sqlTx *sql.Tx
		err   error
	)
	if write {
		sqlTx, err = s.db.BeginTx(ctx, &sql.TxOptions{})
	} else {
		sqlTx, err = s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	}
	if err != nil {
		return err
	}
	// Force the write lock up-front so competing transactions fail fast.
	if write {
		if _, err := sqlTx.Exec(`SELECT 1`); err != nil {
			_ = sqlTx.Rollback()
			return err
		}
	}
	if err := fn(&sqliteTx{tx: sqlTx}); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
}

// sqliteTx implements the Tx interface against a *sql.Tx.
type sqliteTx struct {
	tx *sql.Tx
}

func (t *sqliteTx) GetTask(id string) (*task.InspectionTask, bool) {
	row := t.tx.QueryRow(`SELECT task_id, generation, state, locked_snapshot_json,
		base_lot_id, seal_id, precool_lot, cut_line_id, wash_tank_id, formula_id,
		formula_revision, feed_confirmers_json, anomalies_json, recheck_done,
		final_result, final_credential, created_at_logic, updated_at_logic
		FROM tasks WHERE task_id = ?`, id)
	var (
		ts                                    task.InspectionTask
		snapshotJSON, feedJSON, anomaliesJSON sql.NullString
		finalResult, finalCredential          sql.NullString
	)
	if err := row.Scan(&ts.TaskID, &ts.Generation, &ts.State, &snapshotJSON,
		&ts.BaseLotID, &ts.SealID, &ts.PrecoolLot, &ts.CutLineID, &ts.WashTankID,
		&ts.FormulaID, &ts.FormulaRevision, &feedJSON, &anomaliesJSON, &ts.RecheckDone,
		&finalResult, &finalCredential, &ts.CreatedAtLogic, &ts.UpdatedAtLogic); err != nil {
		return nil, false
	}
	if snapshotJSON.Valid {
		ts.LockedSnapshotJSON = []byte(snapshotJSON.String)
	}
	if feedJSON.Valid {
		_ = json.Unmarshal([]byte(feedJSON.String), &ts.FeedConfirmers)
	}
	if anomaliesJSON.Valid {
		_ = json.Unmarshal([]byte(anomaliesJSON.String), &ts.Anomalies)
	}
	if finalResult.Valid {
		ts.FinalResult = finalResult.String
	}
	if finalCredential.Valid {
		ts.FinalCredential = finalCredential.String
	}
	return &ts, true
}

func (t *sqliteTx) PutTask(ts *task.InspectionTask) error {
	feedJSON, _ := json.Marshal(ts.FeedConfirmers)
	anomaliesJSON, _ := json.Marshal(ts.Anomalies)
	_, err := t.tx.Exec(`INSERT INTO tasks (task_id, generation, state,
		locked_snapshot_json, base_lot_id, seal_id, precool_lot, cut_line_id,
		wash_tank_id, formula_id, formula_revision, feed_confirmers_json,
		anomalies_json, recheck_done, final_result, final_credential,
		created_at_logic, updated_at_logic)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET generation=excluded.generation,
			state=excluded.state, locked_snapshot_json=excluded.locked_snapshot_json,
			base_lot_id=excluded.base_lot_id, seal_id=excluded.seal_id,
			precool_lot=excluded.precool_lot, cut_line_id=excluded.cut_line_id,
			wash_tank_id=excluded.wash_tank_id, formula_id=excluded.formula_id,
			formula_revision=excluded.formula_revision,
			feed_confirmers_json=excluded.feed_confirmers_json,
			anomalies_json=excluded.anomalies_json, recheck_done=excluded.recheck_done,
			final_result=excluded.final_result, final_credential=excluded.final_credential,
			updated_at_logic=excluded.updated_at_logic`,
		ts.TaskID, ts.Generation, ts.State, string(ts.LockedSnapshotJSON),
		ts.BaseLotID, ts.SealID, ts.PrecoolLot, ts.CutLineID, ts.WashTankID,
		ts.FormulaID, ts.FormulaRevision, string(feedJSON), string(anomaliesJSON),
		boolInt(ts.RecheckDone), ts.FinalResult, ts.FinalCredential,
		ts.CreatedAtLogic, ts.UpdatedAtLogic)
	return err
}

func (t *sqliteTx) AcquireLeases(taskID string, generation int, resources []lease.Resource) error {
	for _, r := range resources {
		var existingTask string
		var existingGen int
		err := t.tx.QueryRow(`SELECT task_id, generation FROM leases
			WHERE resource_type = ? AND resource_key = ? AND status = 'acquired'`,
			r.Type, r.Key).Scan(&existingTask, &existingGen)
		if err == nil {
			if existingTask != taskID || existingGen != generation {
				return lease.ErrOccupied
			}
			continue // already held by this task generation
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err := t.tx.Exec(`INSERT INTO leases
			(lease_id, resource_type, resource_key, task_id, generation, status, acquired_at_logic, released_at_logic)
			VALUES (?, ?, ?, ?, ?, 'acquired', 0, 0)`,
			newID("lease"), r.Type, r.Key, taskID, generation); err != nil {
			return err
		}
	}
	return nil
}

func (t *sqliteTx) ReleaseLeases(taskID string, generation int, resources []lease.Resource) error {
	for _, r := range resources {
		if _, err := t.tx.Exec(`UPDATE leases SET status='released' WHERE
			resource_type=? AND resource_key=? AND task_id=? AND generation=?`,
			r.Type, r.Key, taskID, generation); err != nil {
			return err
		}
	}
	return nil
}

func (t *sqliteTx) HeldBy(taskID string) ([]lease.LeaseRecord, error) {
	rows, err := t.tx.Query(`SELECT lease_id, resource_type, resource_key, task_id,
		generation, status, acquired_at_logic, released_at_logic FROM leases
		WHERE task_id = ? AND status = 'acquired' ORDER BY resource_type, resource_key`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []lease.LeaseRecord
	for rows.Next() {
		var l lease.LeaseRecord
		if err := rows.Scan(&l.LeaseID, &l.ResourceType, &l.ResourceKey, &l.TaskID,
			&l.Generation, &l.Status, &l.AcquiredAtLogic, &l.ReleasedAtLogic); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (t *sqliteTx) LeaseFor(resourceType lease.ResourceType, key string) (lease.LeaseRecord, bool, error) {
	var l lease.LeaseRecord
	err := t.tx.QueryRow(`SELECT lease_id, resource_type, resource_key, task_id,
		generation, status, acquired_at_logic, released_at_logic FROM leases
		WHERE resource_type=? AND resource_key=? AND status='acquired'`,
		resourceType, key).Scan(&l.LeaseID, &l.ResourceType, &l.ResourceKey, &l.TaskID,
		&l.Generation, &l.Status, &l.AcquiredAtLogic, &l.ReleasedAtLogic)
	if err == sql.ErrNoRows {
		return l, false, nil
	}
	if err != nil {
		return l, false, err
	}
	return l, true, nil
}

func (t *sqliteTx) Idempotency(operationNo string) (IdempotencyRecord, bool, error) {
	var r IdempotencyRecord
	var body sql.NullString
	err := t.tx.QueryRow(`SELECT operation_no, task_id, generation, operation_kind,
		request_hash, response_code, response_body_json FROM idempotency
		WHERE operation_no = ?`, operationNo).Scan(&r.OperationNo, &r.TaskID,
		&r.Generation, &r.OperationKind, &r.RequestHash, &r.ResponseCode, &body)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	if body.Valid {
		r.ResponseBodyJSON = []byte(body.String)
	}
	return r, true, nil
}

func (t *sqliteTx) PutIdempotency(r IdempotencyRecord) error {
	_, err := t.tx.Exec(`INSERT INTO idempotency (operation_no, task_id, generation,
		operation_kind, request_hash, response_code, response_body_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.OperationNo, r.TaskID, r.Generation, r.OperationKind, r.RequestHash,
		r.ResponseCode, string(r.ResponseBodyJSON))
	return err
}

func (t *sqliteTx) PutCoverageSample(s evidence.CoverageSample) error {
	_, err := t.tx.Exec(`INSERT INTO coverage_samples (task_id, generation,
		sample_time, chlorine_x100, orp_mv, ph_x100, temperature_x100,
		turbidity_x100, source_call_id, valid)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.TaskID, s.Generation, s.SampleTime, s.ChlorineX100, s.ORPMV, s.PHX100,
		s.TemperatureX100, s.TurbidityX100, s.SourceCallID, boolInt(s.Valid))
	return err
}

func (t *sqliteTx) CoverageSamples(taskID string, generation int) ([]evidence.CoverageSample, error) {
	rows, err := t.tx.Query(`SELECT task_id, generation, sample_time, chlorine_x100,
		orp_mv, ph_x100, temperature_x100, turbidity_x100, source_call_id, valid
		FROM coverage_samples WHERE task_id=? AND generation=? ORDER BY sample_time`,
		taskID, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.CoverageSample
	for rows.Next() {
		var s evidence.CoverageSample
		var valid int
		var source sql.NullString
		if err := rows.Scan(&s.TaskID, &s.Generation, &s.SampleTime, &s.ChlorineX100,
			&s.ORPMV, &s.PHX100, &s.TemperatureX100, &s.TurbidityX100, &source, &valid); err != nil {
			return nil, err
		}
		if source.Valid {
			s.SourceCallID = source.String
		}
		s.Valid = valid != 0
		out = append(out, s)
	}
	return out, rows.Err()
}

func (t *sqliteTx) PutEvidence(v evidence.EvidenceVersion) error {
	_, err := t.tx.Exec(`INSERT INTO evidence_versions (evidence_id, task_id,
		generation, kind, blind_code, point_code, plate_well, version_no, raw_json,
		derived_json, accepted, created_at_logic)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.EvidenceID, v.TaskID, v.Generation, v.Kind, v.BlindCode, v.PointCode,
		v.PlateWell, v.VersionNo, string(v.RawJSON), string(v.DerivedJSON),
		boolInt(v.Accepted), v.CreatedAtLogic)
	return err
}

func (t *sqliteTx) Evidence(taskID string, generation int) ([]evidence.EvidenceVersion, error) {
	rows, err := t.tx.Query(`SELECT evidence_id, task_id, generation, kind, blind_code,
		point_code, plate_well, version_no, raw_json, derived_json, accepted,
		created_at_logic FROM evidence_versions WHERE task_id=? AND generation=?
		ORDER BY created_at_logic, version_no`, taskID, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.EvidenceVersion
	for rows.Next() {
		var v evidence.EvidenceVersion
		var raw, derived, blind, point, well sql.NullString
		var accepted int
		if err := rows.Scan(&v.EvidenceID, &v.TaskID, &v.Generation, &v.Kind, &blind,
			&point, &well, &v.VersionNo, &raw, &derived, &accepted, &v.CreatedAtLogic); err != nil {
			return nil, err
		}
		if blind.Valid {
			v.BlindCode = blind.String
		}
		if point.Valid {
			v.PointCode = point.String
		}
		if well.Valid {
			v.PlateWell = well.String
		}
		if raw.Valid {
			v.RawJSON = []byte(raw.String)
		}
		if derived.Valid {
			v.DerivedJSON = []byte(derived.String)
		}
		v.Accepted = accepted != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func (t *sqliteTx) NextEvidenceVersion(taskID string, kind evidence.EvidenceKind, pointCode string) (int, error) {
	var max sql.NullInt64
	err := t.tx.QueryRow(`SELECT MAX(version_no) FROM evidence_versions
		WHERE task_id=? AND kind=? AND point_code=?`, taskID, kind, pointCode).Scan(&max)
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 1, nil
	}
	return int(max.Int64) + 1, nil
}

func (t *sqliteTx) PutAdapterCall(c evidence.AdapterCall) error {
	_, err := t.tx.Exec(`INSERT INTO adapter_calls (call_id, adapter_kind, task_id,
		generation, target_key, attempt_no, script_step, status, error_code,
		next_retry_logic, request_json, response_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CallID, c.AdapterKind, c.TaskID, c.Generation, c.TargetKey, c.AttemptNo,
		c.ScriptStep, c.Status, c.ErrorCode, c.NextRetryLogic, string(c.RequestJSON),
		string(c.ResponseJSON))
	return err
}

func (t *sqliteTx) AdapterCalls(taskID string, generation int) ([]evidence.AdapterCall, error) {
	rows, err := t.tx.Query(`SELECT call_id, adapter_kind, task_id, generation,
		target_key, attempt_no, script_step, status, error_code, next_retry_logic,
		request_json, response_json FROM adapter_calls WHERE task_id=? AND generation=?
		ORDER BY attempt_no, call_id`, taskID, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.AdapterCall
	for rows.Next() {
		var c evidence.AdapterCall
		var target, step, ec sql.NullString
		var req, resp sql.NullString
		if err := rows.Scan(&c.CallID, &c.AdapterKind, &c.TaskID, &c.Generation,
			&target, &c.AttemptNo, &step, &c.Status, &ec, &c.NextRetryLogic, &req, &resp); err != nil {
			return nil, err
		}
		if target.Valid {
			c.TargetKey = target.String
		}
		if step.Valid {
			c.ScriptStep = step.String
		}
		if ec.Valid {
			c.ErrorCode = ec.String
		}
		if req.Valid {
			c.RequestJSON = []byte(req.String)
		}
		if resp.Valid {
			c.ResponseJSON = []byte(resp.String)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (t *sqliteTx) PutReview(r evidence.ReviewDecision) error {
	_, err := t.tx.Exec(`INSERT INTO review_decisions (review_id, task_id,
		generation, reviewer_id, decision, reason_code, created_at_logic)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ReviewID, r.TaskID, r.Generation, r.ReviewerID, r.Decision, r.ReasonCode,
		r.CreatedAtLogic)
	return err
}

func (t *sqliteTx) Reviews(taskID string, generation int) ([]evidence.ReviewDecision, error) {
	rows, err := t.tx.Query(`SELECT review_id, task_id, generation, reviewer_id,
		decision, reason_code, created_at_logic FROM review_decisions
		WHERE task_id=? AND generation=? ORDER BY created_at_logic`, taskID, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.ReviewDecision
	for rows.Next() {
		var r evidence.ReviewDecision
		var reason sql.NullString
		if err := rows.Scan(&r.ReviewID, &r.TaskID, &r.Generation, &r.ReviewerID,
			&r.Decision, &reason, &r.CreatedAtLogic); err != nil {
			return nil, err
		}
		if reason.Valid {
			r.ReasonCode = reason.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (t *sqliteTx) PutAudit(e AuditEvent) error {
	_, err := t.tx.Exec(`INSERT INTO audit_events (task_id, generation, actor_id,
		event_type, reason_code, details_json, logic_time)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.TaskID, e.Generation, e.ActorID, e.EventType, e.ReasonCode,
		string(e.DetailsJSON), e.LogicTime)
	return err
}

func (t *sqliteTx) Audit(taskID string) ([]AuditEvent, error) {
	rows, err := t.tx.Query(`SELECT event_id, task_id, generation, actor_id,
		event_type, reason_code, details_json, logic_time FROM audit_events
		WHERE task_id=? ORDER BY logic_time, event_id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		var actor, reason sql.NullString
		var details sql.NullString
		if err := rows.Scan(&e.EventID, &e.TaskID, &e.Generation, &actor, &e.EventType,
			&reason, &details, &e.LogicTime); err != nil {
			return nil, err
		}
		if actor.Valid {
			e.ActorID = actor.String
		}
		if reason.Valid {
			e.ReasonCode = reason.String
		}
		if details.Valid {
			e.DetailsJSON = []byte(details.String)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (t *sqliteTx) NextLogicTime() (int64, error) {
	if _, err := t.tx.Exec(`UPDATE logic_clock SET value = value + 1 WHERE id = 1`); err != nil {
		return 0, err
	}
	var v int64
	if err := t.tx.QueryRow(`SELECT value FROM logic_clock WHERE id = 1`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, randID())
}

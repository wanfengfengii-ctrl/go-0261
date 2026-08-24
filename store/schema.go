package store

// schema is the SQLite DDL for the LeafWash persistence directory. All tables
// are created idempotently on open so a fresh data directory initializes
// correctly and an existing directory is reused for restart recovery.
const schema = `
CREATE TABLE IF NOT EXISTS tasks (
    task_id              TEXT PRIMARY KEY,
    generation           INTEGER NOT NULL,
    state                TEXT    NOT NULL,
    locked_snapshot_json TEXT,
    base_lot_id          TEXT,
    seal_id              TEXT,
    precool_lot          TEXT,
    cut_line_id          TEXT,
    wash_tank_id         TEXT,
    formula_id           TEXT,
    formula_revision     INTEGER,
    feed_confirmers_json TEXT,
    anomalies_json       TEXT,
    recheck_done         INTEGER NOT NULL DEFAULT 0,
    final_result         TEXT,
    final_credential     TEXT,
    created_at_logic     INTEGER NOT NULL,
    updated_at_logic     INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS leases (
    lease_id          TEXT PRIMARY KEY,
    resource_type     TEXT NOT NULL,
    resource_key      TEXT NOT NULL,
    task_id           TEXT NOT NULL,
    generation        INTEGER NOT NULL,
    status            TEXT NOT NULL,
    acquired_at_logic INTEGER NOT NULL,
    released_at_logic INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_leases_resource ON leases(resource_type, resource_key);

CREATE TABLE IF NOT EXISTS idempotency (
    operation_no       TEXT PRIMARY KEY,
    task_id            TEXT NOT NULL,
    generation         INTEGER NOT NULL,
    operation_kind     TEXT NOT NULL,
    request_hash       TEXT NOT NULL,
    response_code      TEXT NOT NULL,
    response_body_json TEXT
);

CREATE TABLE IF NOT EXISTS coverage_samples (
    task_id          TEXT    NOT NULL,
    generation       INTEGER NOT NULL,
    sample_time      INTEGER NOT NULL,
    chlorine_x100    INTEGER NOT NULL,
    orp_mv           INTEGER NOT NULL,
    ph_x100          INTEGER NOT NULL,
    temperature_x100 INTEGER NOT NULL,
    turbidity_x100   INTEGER NOT NULL,
    source_call_id   TEXT,
    valid            INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_coverage_time ON coverage_samples(task_id, generation, sample_time);

CREATE TABLE IF NOT EXISTS evidence_versions (
    evidence_id      TEXT PRIMARY KEY,
    task_id          TEXT    NOT NULL,
    generation       INTEGER NOT NULL,
    kind             TEXT    NOT NULL,
    blind_code       TEXT,
    point_code       TEXT,
    plate_well       TEXT,
    version_no       INTEGER NOT NULL,
    raw_json         TEXT,
    derived_json     TEXT,
    accepted         INTEGER NOT NULL,
    created_at_logic INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_evidence_task ON evidence_versions(task_id, generation);

CREATE TABLE IF NOT EXISTS adapter_calls (
    call_id          TEXT PRIMARY KEY,
    adapter_kind     TEXT    NOT NULL,
    task_id          TEXT    NOT NULL,
    generation       INTEGER NOT NULL,
    target_key       TEXT,
    attempt_no       INTEGER NOT NULL,
    script_step      TEXT,
    status           TEXT    NOT NULL,
    error_code       TEXT,
    next_retry_logic INTEGER NOT NULL DEFAULT 0,
    request_json     TEXT,
    response_json    TEXT
);
CREATE INDEX IF NOT EXISTS idx_adapter_task ON adapter_calls(task_id, generation);

CREATE TABLE IF NOT EXISTS review_decisions (
    review_id        TEXT PRIMARY KEY,
    task_id          TEXT    NOT NULL,
    generation       INTEGER NOT NULL,
    reviewer_id      TEXT    NOT NULL,
    decision         TEXT    NOT NULL,
    reason_code      TEXT,
    created_at_logic INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_review_task ON review_decisions(task_id, generation);

CREATE TABLE IF NOT EXISTS audit_events (
    event_id     INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id      TEXT    NOT NULL,
    generation   INTEGER NOT NULL,
    actor_id     TEXT,
    event_type   TEXT    NOT NULL,
    reason_code  TEXT,
    details_json TEXT,
    logic_time   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_task ON audit_events(task_id);

CREATE TABLE IF NOT EXISTS logic_clock (
    id     INTEGER PRIMARY KEY CHECK (id = 1),
    value  INTEGER NOT NULL
);
INSERT OR IGNORE INTO logic_clock (id, value) VALUES (1, 0);
`

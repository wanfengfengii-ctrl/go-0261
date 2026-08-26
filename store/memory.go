package store

import (
	"context"
	"sort"
	"sync"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/lease"
	"leafwash-packaging-release-gate/task"
)

// Memory is an in-memory Persistence implementation used by fast unit tests of
// the service layer. It is not durable and not safe for multi-process access;
// the production backend uses SQLite (see OpenSQLite).
type Memory struct {
	mu           sync.Mutex
	cat          catalog.Catalog
	tasks        map[string]*task.InspectionTask
	leases       map[resourceKey]lease.LeaseRecord
	idempotency  map[string]IdempotencyRecord
	coverage     []evidence.CoverageSample
	evidence     []evidence.EvidenceVersion
	adapterCalls []evidence.AdapterCall
	reviews      []evidence.ReviewDecision
	audit        []AuditEvent
	clock        int64
}

type resourceKey struct {
	Type lease.ResourceType
	Key  string
}

// NewMemory constructs an in-memory persistence backend with the given catalog.
func NewMemory(cat catalog.Catalog) *Memory {
	return &Memory{
		cat:         cat,
		tasks:       make(map[string]*task.InspectionTask),
		leases:      make(map[resourceKey]lease.LeaseRecord),
		idempotency: make(map[string]IdempotencyRecord),
	}
}

// Catalog returns the configured read-side catalog.
func (m *Memory) Catalog() catalog.Catalog { return m.cat }

// Health reports liveness (always healthy in memory).
func (m *Memory) Health(context.Context) error { return nil }

// Close is a no-op for the in-memory backend.
func (m *Memory) Close() error { return nil }

// Tx runs fn while holding the store lock. A returned error leaves prior
// writes in place only if fn itself did not mutate state before failing; the
// service layer performs validation before mutation so this matches SQLite
// rollback semantics for the tests that use Memory.
func (m *Memory) Tx(_ context.Context, _ bool, fn func(tx Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(m)
}

// ---- Tasks ----

func (m *Memory) GetTask(id string) (*task.InspectionTask, bool) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	cp := *t
	cp.FeedConfirmers = append([]string(nil), t.FeedConfirmers...)
	cp.Anomalies = append([]string(nil), t.Anomalies...)
	return &cp, true
}

func (m *Memory) PutTask(t *task.InspectionTask) error {
	cp := *t
	cp.FeedConfirmers = append([]string(nil), t.FeedConfirmers...)
	cp.Anomalies = append([]string(nil), t.Anomalies...)
	m.tasks[t.TaskID] = &cp
	return nil
}

// ---- Leases ----

func (m *Memory) AcquireLeases(taskID string, generation int, resources []lease.Resource) error {
	for _, r := range resources {
		k := resourceKey{Type: r.Type, Key: r.Key}
		if existing, ok := m.leases[k]; ok && (existing.TaskID != taskID || existing.Generation != generation) {
			return lease.ErrOccupied
		}
	}
	for _, r := range resources {
		k := resourceKey{Type: r.Type, Key: r.Key}
		m.leases[k] = lease.LeaseRecord{
			LeaseID:      newID("lease"),
			ResourceType: r.Type,
			ResourceKey:  r.Key,
			TaskID:       taskID,
			Generation:   generation,
			Status:       lease.StatusAcquired,
		}
	}
	return nil
}

func (m *Memory) ReleaseLeases(taskID string, generation int, resources []lease.Resource) error {
	for _, r := range resources {
		k := resourceKey{Type: r.Type, Key: r.Key}
		if l, ok := m.leases[k]; ok && l.TaskID == taskID && l.Generation == generation {
			l.Status = lease.StatusReleased
			m.leases[k] = l
		}
	}
	return nil
}

func (m *Memory) HeldBy(taskID string) ([]lease.LeaseRecord, error) {
	var out []lease.LeaseRecord
	for _, l := range m.leases {
		if l.TaskID == taskID && l.Status == lease.StatusAcquired {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ResourceType != out[j].ResourceType {
			return out[i].ResourceType < out[j].ResourceType
		}
		return out[i].ResourceKey < out[j].ResourceKey
	})
	return out, nil
}

func (m *Memory) LeaseFor(resourceType lease.ResourceType, key string) (lease.LeaseRecord, bool, error) {
	l, ok := m.leases[resourceKey{Type: resourceType, Key: key}]
	if !ok || l.Status != lease.StatusAcquired {
		return l, false, nil
	}
	return l, true, nil
}

// ---- Idempotency ----

func (m *Memory) Idempotency(operationNo string) (IdempotencyRecord, bool, error) {
	r, ok := m.idempotency[operationNo]
	if !ok {
		return r, false, nil
	}
	r.ResponseBodyJSON = append([]byte(nil), r.ResponseBodyJSON...)
	return r, true, nil
}

func (m *Memory) PutIdempotency(r IdempotencyRecord) error {
	r.ResponseBodyJSON = append([]byte(nil), r.ResponseBodyJSON...)
	m.idempotency[r.OperationNo] = r
	return nil
}

// ---- Coverage samples ----

func (m *Memory) PutCoverageSample(s evidence.CoverageSample) error {
	m.coverage = append(m.coverage, s)
	return nil
}

func (m *Memory) CoverageSamples(taskID string, generation int) ([]evidence.CoverageSample, error) {
	var out []evidence.CoverageSample
	for _, s := range m.coverage {
		if s.TaskID == taskID && s.Generation == generation {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SampleTime < out[j].SampleTime })
	return out, nil
}

// ---- Evidence ----

func (m *Memory) PutEvidence(v evidence.EvidenceVersion) error {
	m.evidence = append(m.evidence, v)
	return nil
}

func (m *Memory) Evidence(taskID string, generation int) ([]evidence.EvidenceVersion, error) {
	var out []evidence.EvidenceVersion
	for _, v := range m.evidence {
		if v.TaskID == taskID && v.Generation == generation {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAtLogic != out[j].CreatedAtLogic {
			return out[i].CreatedAtLogic < out[j].CreatedAtLogic
		}
		return out[i].VersionNo < out[j].VersionNo
	})
	return out, nil
}

func (m *Memory) NextEvidenceVersion(taskID string, kind evidence.EvidenceKind, pointCode string) (int, error) {
	max := 0
	for _, v := range m.evidence {
		if v.TaskID == taskID && v.Kind == kind && v.PointCode == pointCode && v.VersionNo > max {
			max = v.VersionNo
		}
	}
	return max + 1, nil
}

// ---- Adapter calls ----

func (m *Memory) PutAdapterCall(c evidence.AdapterCall) error {
	m.adapterCalls = append(m.adapterCalls, c)
	return nil
}

func (m *Memory) AdapterCalls(taskID string, generation int) ([]evidence.AdapterCall, error) {
	var out []evidence.AdapterCall
	for _, c := range m.adapterCalls {
		if c.TaskID == taskID && c.Generation == generation {
			out = append(out, c)
		}
	}
	return out, nil
}

// ---- Reviews ----

func (m *Memory) PutReview(r evidence.ReviewDecision) error {
	m.reviews = append(m.reviews, r)
	return nil
}

func (m *Memory) Reviews(taskID string, generation int) ([]evidence.ReviewDecision, error) {
	var out []evidence.ReviewDecision
	for _, r := range m.reviews {
		if r.TaskID == taskID && r.Generation == generation {
			out = append(out, r)
		}
	}
	return out, nil
}

// ---- Audit ----

func (m *Memory) PutAudit(e AuditEvent) error {
	e.DetailsJSON = append([]byte(nil), e.DetailsJSON...)
	m.audit = append(m.audit, e)
	return nil
}

func (m *Memory) Audit(taskID string) ([]AuditEvent, error) {
	var out []AuditEvent
	for _, e := range m.audit {
		if e.TaskID == taskID {
			out = append(out, e)
		}
	}
	return out, nil
}

// ---- Logic clock ----

func (m *Memory) NextLogicTime() (int64, error) {
	m.clock++
	return m.clock, nil
}

// Package lease implements the "槽位/板孔/沥水时隙租约登记册" (tank/well/drain
// slot lease registry) component. It owns the exclusive-occupancy records for
// wash tanks, culture-plate wells, centrifuge drain slots, and sampling blind
// codes within open tasks, and the resource types used by transactional
// acquisition and restart recovery.
package lease

import "errors"

// ErrOccupied is returned when a requested resource is already held by a
// different open task generation.
var ErrOccupied = errors.New("resource already occupied")

// ResourceType enumerates the exclusive resources tracked by the registry.
type ResourceType string

const (
	ResourceWashTank  ResourceType = "wash_tank"  // 清洗槽
	ResourcePlateWell ResourceType = "plate_well" // 培养板孔位
	ResourceDrainSlot ResourceType = "drain_slot" // 离心沥水时隙
	ResourceBlindCode ResourceType = "blind_code" // 采样盲码
)

// IsValid reports whether t is a known resource type.
func (t ResourceType) IsValid() bool {
	switch t {
	case ResourceWashTank, ResourcePlateWell, ResourceDrainSlot, ResourceBlindCode:
		return true
	}
	return false
}

// Status is the lifecycle status of a lease record.
type Status string

const (
	StatusAcquired Status = "acquired"
	StatusReleased Status = "released"
)

// ResourceKey uniquely identifies a resource of a given type (e.g. a tank id,
// a well code, a drain time-slot, or a blind code).
type ResourceKey = string

// LeaseRecord is a single exclusive occupancy bound to one open task generation.
type LeaseRecord struct {
	LeaseID         string       `json:"lease_id"`
	ResourceType    ResourceType `json:"resource_type"`
	ResourceKey     ResourceKey  `json:"resource_key"`
	TaskID          string       `json:"task_id"`
	Generation      int          `json:"generation"`
	Status          Status       `json:"status"`
	AcquiredAtLogic int64        `json:"acquired_at_logic"`
	ReleasedAtLogic int64        `json:"released_at_logic"`
}

// Registry is the transaction-scoped occupancy contract. All mutations happen
// inside a single SQLite transaction so that a failed acquisition leaves no
// partial occupancy (failure boundary #1).
type Registry interface {
	// Acquire atomically claims each resource for the task generation. It
	// returns an error (and rolls back) if any resource is already held.
	Acquire(taskID string, generation int, resources []Resource) error
	// Release frees the given resources held by the task generation.
	Release(taskID string, generation int, resources []Resource) error
	// HeldBy reports which resources are currently acquired, keyed by task.
	HeldBy(taskID string) ([]LeaseRecord, error)
}

// Resource is a type/key pair submitted for acquisition.
type Resource struct {
	Type ResourceType `json:"resource_type"`
	Key  ResourceKey  `json:"resource_key"`
}

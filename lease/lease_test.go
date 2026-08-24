package lease

import "testing"

func TestResourceTypeValid(t *testing.T) {
	for _, r := range []ResourceType{ResourceWashTank, ResourcePlateWell, ResourceDrainSlot, ResourceBlindCode} {
		if !r.IsValid() {
			t.Errorf("%q should be valid", r)
		}
	}
	if (ResourceType("unknown")).IsValid() {
		t.Error("unknown resource type must be invalid")
	}
}

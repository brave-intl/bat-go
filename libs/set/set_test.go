package set

import (
	"testing"
)

func TestUnsafeSliceSet(t *testing.T) {
	set := NewUnsafeSliceSet()
	if r, err := set.Add("FOO"); err != nil || !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, err := set.Contains("FOO"); err != nil || !r {
		t.Error("Set should contain last added element")
	}

	if r, err := set.Add("FOO"); err != nil || r {
		t.Error("Re-add of same element should fail")
	}

	if r, err := set.Add("BAR"); err != nil || !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, err := set.Contains("BAR"); err != nil || !r {
		t.Error("Set should contain last added element")
	}
	if r, err := set.Contains("FOO"); err != nil || !r {
		t.Error("Set should contain all added elements")
	}

	if r, err := set.Cardinality(); err != nil || r != 2 {
		t.Error("Set should contain contain 2 elements")
	}
}

func TestSliceSet(t *testing.T) {
	set := NewSliceSet()
	if r, err := set.Add("FOO"); err != nil || !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, err := set.Contains("FOO"); err != nil || !r {
		t.Error("Set should contain last added element")
	}

	if r, err := set.Add("FOO"); err != nil || r {
		t.Error("Re-add of same element should fail")
	}

	if r, err := set.Add("BAR"); err != nil || !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, err := set.Contains("BAR"); err != nil || !r {
		t.Error("Set should contain last added element")
	}
	if r, err := set.Contains("FOO"); err != nil || !r {
		t.Error("Set should contain all added elements")
	}

	if r, err := set.Cardinality(); err != nil || r != 2 {
		t.Error("Set should contain contain 2 elements")
	}
}

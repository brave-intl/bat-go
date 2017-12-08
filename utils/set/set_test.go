package set

import (
	"testing"
)

func TestUnsafeSliceSet(t *testing.T) {
	set := NewUnsafeSliceSet()
	if r, _ := set.Add("FOO"); !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, _ := set.Contains("FOO"); !r {
		t.Error("Set should contain last added element")
	}

	if r, _ := set.Add("FOO"); r {
		t.Error("Re-add of same element should fail")
	}

	if r, _ := set.Add("BAR"); !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, _ := set.Contains("BAR"); !r {
		t.Error("Set should contain last added element")
	}
	if r, _ := set.Contains("FOO"); !r {
		t.Error("Set should contain all added elements")
	}

	if r, _ := set.Cardinality(); r != 2 {
		t.Error("Set should contain contain 2 elements")
	}
}

func TestSliceSet(t *testing.T) {
	set := NewSliceSet()
	if r, _ := set.Add("FOO"); !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, _ := set.Contains("FOO"); !r {
		t.Error("Set should contain last added element")
	}

	if r, _ := set.Add("FOO"); r {
		t.Error("Re-add of same element should fail")
	}

	if r, _ := set.Add("BAR"); !r {
		t.Error("Add to empty set should always succeed")
	}
	if r, _ := set.Contains("BAR"); !r {
		t.Error("Set should contain last added element")
	}
	if r, _ := set.Contains("FOO"); !r {
		t.Error("Set should contain all added elements")
	}

	if r, _ := set.Cardinality(); r != 2 {
		t.Error("Set should contain contain 2 elements")
	}
}

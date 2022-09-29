package custodian

import "testing"

func TestVerdictAllowList(t *testing.T) {
	gabm := GeoAllowBlockMap{
		Allow: []string{"US", "FR"},
	}
	if gabm.Verdict("CA") {
		t.Error("should have failed, CA not in allow list")
	}

	if !gabm.Verdict("US") {
		t.Error("should have passed, US in allow list")
	}
}

func TestVerdictBlockList(t *testing.T) {
	gabm := GeoAllowBlockMap{
		Block: []string{"US", "FR"},
	}
	if !gabm.Verdict("CA") {
		t.Error("should have been true, CA not in block list")
	}

	if gabm.Verdict("US") {
		t.Error("should have been false, US in block list")
	}
}

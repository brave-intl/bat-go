package custodian

import "testing"

func TestVerdict(t *testing.T) {
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

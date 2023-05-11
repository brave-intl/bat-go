package kv

import "testing"

func TestMapKv(t *testing.T) {
	store := UnsafeMapKv{m: map[string]string{}}

	if r, err := store.Set("FOO", "BAR", -1, true); !r || err != nil {
		t.Error("Set to empty kv store should always succeed")
	}
	if r, err := store.Get("FOO"); r != "BAR" || err != nil {
		t.Error("Get should return added value")
	}

	if r, err := store.Set("FOO", "CAFE", -1, true); !r || err != nil {
		t.Error("Set with upsert should succeed")
	}
	if r, err := store.Get("FOO"); r != "CAFE" || err != nil {
		t.Error("Get should return last added value")
	}

	if r, err := store.Set("FOO", "DEAD", -1, false); r || err == nil {
		t.Error("Add without upsert should fail")
	}
	if r, err := store.Get("FOO"); r != "CAFE" || err != nil {
		t.Error("Get should return last successfully added value")
	}

	if _, err := store.Get("DEAD"); err == nil {
		t.Error("Get should return an error on nonexistant key")
	}

	if r, err := store.Delete("FOO"); !r || err != nil {
		t.Error("Delete should return true on success")
	}
	if r, err := store.Delete("FOO"); r || err != nil {
		t.Error("Delete should return false when the key does not exist")
	}
}

// +build integration

package datastore

import (
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
)

func TestRedisSet(t *testing.T) {
	c, err := redis.Dial("tcp", ":6379")
	if err != nil {
		t.Error(err)
	}
	c.Do("SELECT", "42")
	c.Do("FLUSHDB")

	set := GetRedisSet(&c, "testing:set")
	if r, _ := set.Add("FOO"); r != true {
		t.Error("Add to empty set should always succeed")
	}
	if r, _ := set.Contains("FOO"); r != true {
		t.Error("Set should contain last added element")
	}

	if r, _ := set.Add("FOO"); r != false {
		t.Error("Re-add of same element should fail")
	}

	if r, _ := set.Add("BAR"); r != true {
		t.Error("Add to empty set should always succeed")
	}
	if r, _ := set.Contains("BAR"); r != true {
		t.Error("Set should contain last added element")
	}
	if r, _ := set.Contains("FOO"); r != true {
		t.Error("Set should contain all added elements")
	}

	if r, _ := set.Cardinality(); r != 2 {
		t.Error("Set should contain contain 2 elements")
	}
}

func TestRedisKv(t *testing.T) {
	c, err := redis.Dial("tcp", ":6379")
	if err != nil {
		t.Error(err)
	}
	c.Do("SELECT", "42")
	c.Do("FLUSHDB")

	store := GetRedisKv(&c)

	if r, err := store.Set("FOO", "BAR", -1, true); r != true || err != nil {
		t.Error("Set to empty kv store should always succeed")
	}
	if r, err := store.Get("FOO"); r != "BAR" || err != nil {
		t.Error("Get should return added value")
	}

	if r, err := store.Set("FOO", "CAFE", -1, true); r != true || err != nil {
		t.Error("Set with upsert should succeed")
	}
	if r, err := store.Get("FOO"); r != "CAFE" || err != nil {
		t.Error("Get should return last added value")
	}

	if r, err := store.Set("FOO", "DEAD", -1, false); r != false || err == nil {
		t.Error("Add without upsert should fail")
	}
	if r, err := store.Get("FOO"); r != "CAFE" || err != nil {
		t.Error("Get should return last successfully added value")
	}

	if _, err := store.Get("DEAD"); err == nil {
		t.Error("Get should return an error on nonexistant key")
	}

	if r, err := store.Delete("FOO"); r != true || err != nil {
		t.Error("Delete should return true on success")
	}
	if r, err := store.Delete("FOO"); r != false || err != nil {
		t.Error("Delete should return false when the key does not exist")
	}

	if r, err := store.Set("FOO", "BAR", 1, false); r != true || err != nil {
		t.Error("Set with ttl should succeed")
	}
	time.Sleep(2 * time.Second)
	if _, err := store.Get("FOO"); err == nil {
		t.Error("Get should return an error on expired key")
	}
	if r, err := store.Set("FOO", "DEAD", -1, false); r != true || err != nil {
		t.Error("Add without upsert should succeed on expired key")
	}
}

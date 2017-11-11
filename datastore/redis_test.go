// +build integration

package datastore

import (
	"github.com/garyburd/redigo/redis"
	"testing"
)

func TestRedisSet(t *testing.T) {
	c, err := redis.Dial("tcp", ":6379")
	if err != nil {
		t.Error(err)
	}
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

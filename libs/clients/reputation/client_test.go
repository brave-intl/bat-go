package reputation

import (
	"fmt"
	"testing"
)

func TestLinkingReputableRequestParam(t *testing.T) {
	r := &IsLinkingReputableRequestQSB{
		Country: "US",
	}
	v, err := r.GenerateQueryString()
	if err != nil {
		t.Error("error: ", err)
	}

	if v.Encode() != "country=US" {
		fmt.Println(v.Encode())
		t.Error("query string is not correct")
	}
}

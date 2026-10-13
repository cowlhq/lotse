package protocol

import "testing"

func TestSubprotocol(t *testing.T) {
	for _, v := range Versions {
		got, err := ParseSubprotocol(Subprotocol(v))
		if err != nil || got != v {
			t.Errorf("version %d: got %d, %v", v, got, err)
		}
	}
	for _, s := range []string{"", "lotse.", "lotse.x", "other.1", "1"} {
		if _, err := ParseSubprotocol(s); err == nil {
			t.Errorf("%q was taken for a lotse protocol", s)
		}
	}
}

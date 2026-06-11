package hashid

import "testing"

func TestNew(t *testing.T) {
	id := New("playlist/series")
	if len(id) != 16 {
		t.Errorf("expected 16 hex chars, got %q (%d)", id, len(id))
	}
	if New("playlist/series") != id {
		t.Error("same input must produce the same id")
	}
	if New("playlist/movies") == id {
		t.Error("distinct inputs must produce distinct ids")
	}
	// Multi-part input is delimited: ("ab","c") and ("a","bc") must differ.
	if New("ab", "c") == New("a", "bc") {
		t.Error("part boundaries must affect the id")
	}
}

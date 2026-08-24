package ulid

import (
	"sort"
	"testing"
	"time"
)

func TestNewHasFixedLength(t *testing.T) {
	id := New()
	if len(id) != 26 {
		t.Fatalf("length = %d, want 26: %q", len(id), id)
	}
}

func TestSameMillisecondStaysOrdered(t *testing.T) {
	at := time.UnixMilli(1755900000000)
	ids := make([]string, 200)
	for i := range ids {
		ids[i] = newAt(at)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("identifiers from one millisecond are not sorted")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate identifier %q", id)
		}
		seen[id] = true
	}
}

func TestLaterTimeSortsLater(t *testing.T) {
	early := newAt(time.UnixMilli(1000))
	late := newAt(time.UnixMilli(2000))
	if !(early < late) {
		t.Fatalf("%q is not before %q", early, late)
	}
}

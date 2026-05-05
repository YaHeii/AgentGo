package provider

import "testing"

func TestStreamEventTypesAreStable(t *testing.T) {
	t.Parallel()

	if StreamEventDelta != "delta" {
		t.Fatalf("unexpected delta type: %q", StreamEventDelta)
	}
	if StreamEventDone != "done" {
		t.Fatalf("unexpected done type: %q", StreamEventDone)
	}
}

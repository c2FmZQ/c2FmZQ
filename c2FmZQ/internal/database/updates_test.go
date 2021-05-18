package database

import (
	"testing"
)

func TestPruneDeleteEvents(t *testing.T) {
	var horizon int64
	events := []DeleteEvent{
		{File: "one", Date: 1000},
		{File: "two", Date: 2000},
		{File: "three", Date: 3000},
		{File: "four", Date: 4000},
	}

	CurrentTimeForTesting = 5000
	pruneDeleteEvents(&events, &horizon)
	t.Logf("events@%d: %#v", CurrentTimeForTesting, events)
	if want, got := 4, len(events); want != got {
		t.Errorf("Unexpected changed to the delete events. Want %d, got %d", want, got)
	}
	if want, got := int64(0), horizon; want != got {
		t.Errorf("Unexpected changed to the delete horizon. Want %d, got %d", want, got)
	}

	CurrentTimeForTesting = 1000 + 180*24*60*60*1000
	pruneDeleteEvents(&events, &horizon)
	t.Logf("events@%d: %#v", CurrentTimeForTesting, events)
	if want, got := 4, len(events); want != got {
		t.Errorf("Unexpected changed to the delete events. Want %d, got %d", want, got)
	}
	if want, got := int64(0), horizon; want != got {
		t.Errorf("Unexpected changed to the delete horizon. Want %d, got %d", want, got)
	}

	CurrentTimeForTesting = 1001 + 180*24*60*60*1000
	pruneDeleteEvents(&events, &horizon)
	t.Logf("events@%d: %#v", CurrentTimeForTesting, events)
	if want, got := 3, len(events); want != got {
		t.Errorf("Unexpected changed to the delete events. Want %d, got %d", want, got)
	}
	if want, got := int64(1001), horizon; want != got {
		t.Errorf("Unexpected changed to the delete horizon. Want %d, got %d", want, got)
	}

	CurrentTimeForTesting = 4001 + 180*24*60*60*1000
	pruneDeleteEvents(&events, &horizon)
	t.Logf("events@%d: %#v", CurrentTimeForTesting, events)
	if want, got := 0, len(events); want != got {
		t.Errorf("Unexpected changed to the delete events. Want %d, got %d", want, got)
	}
	if want, got := int64(4001), horizon; want != got {
		t.Errorf("Unexpected changed to the delete horizon. Want %d, got %d", want, got)
	}
}

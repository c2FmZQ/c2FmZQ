//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

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

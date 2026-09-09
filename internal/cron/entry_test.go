package cron

import "testing"

func TestParseEntryValid(t *testing.T) {
	data := []byte("name: nightly-report\nflow: report\nschedule: \"0 6 * * *\"\ncreated_by: simon\n")
	e, err := ParseEntry(data)
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "nightly-report" || e.Flow != "report" || e.Schedule != "0 6 * * *" {
		t.Errorf("e = %+v", e)
	}
}

func TestParseEntryRejectsNoName(t *testing.T) {
	data := []byte("flow: report\nschedule: \"* * * * *\"\n")
	if _, err := ParseEntry(data); err == nil {
		t.Fatal("expected an error for a missing name")
	}
}

func TestParseEntryRejectsNoFlow(t *testing.T) {
	data := []byte("name: x\nschedule: \"* * * * *\"\n")
	if _, err := ParseEntry(data); err == nil {
		t.Fatal("expected an error for a missing flow")
	}
}

func TestParseEntryRejectsAnInvalidSchedule(t *testing.T) {
	data := []byte("name: x\nflow: report\nschedule: \"nonsense\"\n")
	if _, err := ParseEntry(data); err == nil {
		t.Fatal("expected an error for an invalid schedule")
	}
}

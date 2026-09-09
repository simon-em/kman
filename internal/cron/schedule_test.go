package cron

import (
	"testing"
	"time"
)

func mustParseSchedule(t *testing.T, expr string) Schedule {
	t.Helper()
	s, err := ParseSchedule(expr)
	if err != nil {
		t.Fatalf("ParseSchedule(%q): %v", expr, err)
	}
	return s
}

func TestParseScheduleRejectsWrongFieldCount(t *testing.T) {
	if _, err := ParseSchedule("* * * *"); err == nil {
		t.Fatal("expected an error for a 4-field schedule")
	}
}

func TestParseScheduleRejectsAnOutOfRangeValue(t *testing.T) {
	if _, err := ParseSchedule("60 * * * *"); err == nil {
		t.Fatal("expected an error for minute=60")
	}
}

func TestParseScheduleRejectsAnInvalidStep(t *testing.T) {
	if _, err := ParseSchedule("*/0 * * * *"); err == nil {
		t.Fatal("expected an error for a zero step")
	}
}

func TestScheduleEveryMinuteMatchesAnything(t *testing.T) {
	s := mustParseSchedule(t, "* * * * *")
	if !s.Matches(time.Date(2026, 3, 5, 13, 47, 0, 0, time.UTC)) {
		t.Error("expected * * * * * to match")
	}
}

func TestScheduleExactMinuteAndHour(t *testing.T) {
	s := mustParseSchedule(t, "30 9 * * *")
	if !s.Matches(time.Date(2026, 3, 5, 9, 30, 0, 0, time.UTC)) {
		t.Error("expected 9:30 to match")
	}
	if s.Matches(time.Date(2026, 3, 5, 9, 31, 0, 0, time.UTC)) {
		t.Error("expected 9:31 not to match")
	}
}

func TestScheduleStepValue(t *testing.T) {
	s := mustParseSchedule(t, "*/15 * * * *")
	for _, m := range []int{0, 15, 30, 45} {
		if !s.Matches(time.Date(2026, 3, 5, 9, m, 0, 0, time.UTC)) {
			t.Errorf("expected minute %d to match */15", m)
		}
	}
	if s.Matches(time.Date(2026, 3, 5, 9, 10, 0, 0, time.UTC)) {
		t.Error("expected minute 10 not to match */15")
	}
}

func TestScheduleCommaList(t *testing.T) {
	s := mustParseSchedule(t, "0 9,17 * * *")
	if !s.Matches(time.Date(2026, 3, 5, 9, 0, 0, 0, time.UTC)) {
		t.Error("expected hour 9 to match")
	}
	if !s.Matches(time.Date(2026, 3, 5, 17, 0, 0, 0, time.UTC)) {
		t.Error("expected hour 17 to match")
	}
	if s.Matches(time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)) {
		t.Error("expected hour 12 not to match")
	}
}

func TestScheduleRange(t *testing.T) {
	s := mustParseSchedule(t, "0 9-11 * * *")
	for _, h := range []int{9, 10, 11} {
		if !s.Matches(time.Date(2026, 3, 5, h, 0, 0, 0, time.UTC)) {
			t.Errorf("expected hour %d to match 9-11", h)
		}
	}
	if s.Matches(time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)) {
		t.Error("expected hour 12 not to match 9-11")
	}
}

func TestScheduleWeekday(t *testing.T) {
	s := mustParseSchedule(t, "0 9 * * 1-5")
	monday := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	if !s.Matches(monday) {
		t.Error("expected Monday to match a weekday schedule")
	}
	if s.Matches(sunday) {
		t.Error("expected Sunday not to match a weekday schedule")
	}
}

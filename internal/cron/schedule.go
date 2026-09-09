package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type field struct {
	values map[int]bool
}

func (f field) has(v int) bool { return f.values[v] }

type Schedule struct {
	minute, hour, dom, month, dow field
}

func ParseSchedule(expr string) (Schedule, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("schedule %q: want 5 fields (minute hour dom month dow), got %d", expr, len(parts))
	}
	minute, err := parseField(parts[0], 0, 59)
	if err != nil {
		return Schedule{}, err
	}
	hour, err := parseField(parts[1], 0, 23)
	if err != nil {
		return Schedule{}, err
	}
	dom, err := parseField(parts[2], 1, 31)
	if err != nil {
		return Schedule{}, err
	}
	month, err := parseField(parts[3], 1, 12)
	if err != nil {
		return Schedule{}, err
	}
	dow, err := parseField(parts[4], 0, 6)
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{minute: minute, hour: hour, dom: dom, month: month, dow: dow}, nil
}

func (s Schedule) Matches(t time.Time) bool {
	return s.minute.has(t.Minute()) && s.hour.has(t.Hour()) &&
		s.dom.has(t.Day()) && s.month.has(int(t.Month())) && s.dow.has(int(t.Weekday()))
}

func parseField(expr string, min, max int) (field, error) {
	f := field{values: map[int]bool{}}
	for _, part := range strings.Split(expr, ",") {
		step := 1
		base := part
		if idx := strings.Index(part, "/"); idx >= 0 {
			base = part[:idx]
			n, err := strconv.Atoi(part[idx+1:])
			if err != nil || n <= 0 {
				return field{}, fmt.Errorf("invalid step in %q", part)
			}
			step = n
		}
		lo, hi := min, max
		if base != "*" {
			if dash := strings.Index(base, "-"); dash >= 0 {
				var err error
				lo, err = strconv.Atoi(base[:dash])
				if err != nil {
					return field{}, fmt.Errorf("invalid range in %q", part)
				}
				hi, err = strconv.Atoi(base[dash+1:])
				if err != nil {
					return field{}, fmt.Errorf("invalid range in %q", part)
				}
			} else {
				n, err := strconv.Atoi(base)
				if err != nil {
					return field{}, fmt.Errorf("invalid value %q", base)
				}
				lo, hi = n, n
			}
		}
		if lo < min || hi > max || lo > hi {
			return field{}, fmt.Errorf("%q out of range [%d,%d]", part, min, max)
		}
		for v := lo; v <= hi; v += step {
			f.values[v] = true
		}
	}
	return f, nil
}

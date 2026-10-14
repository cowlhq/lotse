package config

import (
	"fmt"
	"strings"
	"time"
)

// Schedule is a window that comes back: daily, or on some days of the
// week, such as "sun 02:00-04:00", "mon-fri 22:00-02:00" or
// "daily 04:00-06:00". A window that runs over midnight belongs to the day
// it starts on.
type Schedule struct {
	// Days are the weekdays it starts on, by time.Weekday.
	Days       [7]bool
	Start, End int // minutes after midnight
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// ParseSchedule reads a schedule: days, then a window. Without days, or
// with "daily", it is every day.
func ParseSchedule(s string) (Schedule, error) {
	var sc Schedule
	fields := strings.Fields(strings.ToLower(s))
	bad := fmt.Errorf("a schedule such as \"daily 04:00-06:00\", \"sun 02:00-04:00\" or \"mon-fri 22:00-23:00\"")
	switch len(fields) {
	case 1:
		fields = []string{"daily", fields[0]}
	case 2:
	default:
		return sc, bad
	}
	var err error
	if sc.Start, sc.End, err = ParseWindow(fields[1]); err != nil {
		return sc, bad
	}
	if fields[0] == "daily" {
		sc.Days = [7]bool{true, true, true, true, true, true, true}
		return sc, nil
	}
	for _, part := range strings.Split(fields[0], ",") {
		from, to, isRange := strings.Cut(part, "-")
		a, ok1 := weekdays[from]
		b, ok2 := weekdays[to]
		if !ok1 || isRange && !ok2 {
			return sc, bad
		}
		if !isRange {
			b = a
		}
		for d := a; ; d = (d + 1) % 7 {
			sc.Days[d] = true
			if d == b {
				break
			}
		}
	}
	return sc, nil
}

// Contains says whether t falls into the schedule.
func (sc Schedule) Contains(t time.Time) bool {
	m := t.Hour()*60 + t.Minute()
	today, yesterday := sc.Days[t.Weekday()], sc.Days[(t.Weekday()+6)%7]
	if sc.Start < sc.End {
		return today && m >= sc.Start && m < sc.End
	}
	return today && m >= sc.Start || yesterday && m < sc.End
}

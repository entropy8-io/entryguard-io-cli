package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SessionDuration is a parsed --duration value. Exactly one of Hours or Minutes is set,
// matching the API's mutually exclusive durationHours / durationMinutes fields.
type SessionDuration struct {
	Hours   *int
	Minutes *int
}

// ParseSessionDuration accepts the forms the CLI has always taken plus Go-style units:
//
//	"4"      -> 4 hours (the original integer-hours flag)
//	"4h"     -> 4 hours
//	"15m"    -> 15 minutes (minimum 5)
//	"1h30m"  -> 90 minutes
//
// Whole hours are sent as durationHours so older servers keep working; anything
// else is sent as durationMinutes. An empty string means "server default".
func ParseSessionDuration(s string) (SessionDuration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return SessionDuration{}, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return SessionDuration{}, fmt.Errorf("duration must be positive, got %q", s)
		}
		return SessionDuration{Hours: &n}, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return SessionDuration{}, fmt.Errorf("invalid duration %q: use hours (4, 4h) or minutes (15m, 1h30m)", s)
	}
	if d <= 0 {
		return SessionDuration{}, fmt.Errorf("duration must be positive, got %q", s)
	}
	if d%time.Hour == 0 {
		h := int(d / time.Hour)
		return SessionDuration{Hours: &h}, nil
	}
	if d%time.Minute != 0 {
		return SessionDuration{}, fmt.Errorf("duration %q must be whole minutes", s)
	}
	m := int(d / time.Minute)
	if m < 5 {
		return SessionDuration{}, fmt.Errorf("duration must be at least 5 minutes, got %q", s)
	}
	return SessionDuration{Minutes: &m}, nil
}

// Apply writes the duration into a start request.
func (d SessionDuration) Apply(req *StartSessionRequest) {
	req.DurationHours = d.Hours
	req.DurationMinutes = d.Minutes
}

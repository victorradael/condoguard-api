// Package civil handles calendar days (no time of day), which is how unit
// links start and end. A day is a day in the condominium's time zone.
package civil

import (
	"errors"
	"time"
)

// Layout is the wire format of a day: 2026-11-10.
const Layout = "2006-01-02"

// ErrInvalid is returned for text that is not a YYYY-MM-DD day.
var ErrInvalid = errors.New("date must be YYYY-MM-DD")

var zone = func() *time.Location {
	if l, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return l
	}
	return time.FixedZone("BRT", -3*60*60)
}()

// Today returns the current day (at 00:00 UTC, so it compares with parsed days).
func Today() time.Time { return Day(time.Now().In(zone)) }

// Day drops the time of day of t, keeping the calendar day it shows.
func Day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Parse reads a YYYY-MM-DD day.
func Parse(s string) (time.Time, error) {
	t, err := time.Parse(Layout, s)
	if err != nil {
		return time.Time{}, ErrInvalid
	}
	return t, nil
}

// Format writes a day as YYYY-MM-DD.
func Format(t time.Time) string { return t.Format(Layout) }

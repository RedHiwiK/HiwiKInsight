// Package tz holds the reporting time zone of a deployment.
//
// Every "day" in HiwiKInsight (daily active users, cohorts, report periods,
// alert windows) is a calendar day in this zone. It is set once at startup
// from the config file; tests set it explicitly.
//
// The package also registers SQLite functions so SQL never hard-codes a UTC
// offset (and stays correct across daylight-saving changes):
//
//	local_date(ms)      -> 'YYYY-MM-DD'          for a millisecond Unix timestamp
//	local_datetime(ms)  -> 'YYYY-MM-DD HH:MM:SS'
//	local_today()       -> today's date in the reporting zone
package tz

import (
	"database/sql/driver"
	"fmt"
	"sync/atomic"
	"time"
	_ "time/tzdata" // static binaries and slim containers may lack a zoneinfo database

	"modernc.org/sqlite"
)

var loc atomic.Pointer[time.Location]

func init() {
	loc.Store(time.UTC)
	sqlite.MustRegisterScalarFunction("local_date", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		ms, ok := toInt64(args[0])
		if !ok {
			return nil, nil
		}
		return Day(ms), nil
	})
	sqlite.MustRegisterScalarFunction("local_datetime", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		ms, ok := toInt64(args[0])
		if !ok {
			return nil, nil
		}
		return time.UnixMilli(ms).In(Location()).Format("2006-01-02 15:04:05"), nil
	})
	sqlite.MustRegisterScalarFunction("local_today", 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		return Today(), nil
	})
}

// Set changes the reporting time zone, e.g. "Asia/Shanghai" or "America/New_York".
func Set(name string) error {
	l, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("timezone %q: %w", name, err)
	}
	loc.Store(l)
	return nil
}

// MustSet is Set for tests and fixed defaults.
func MustSet(name string) {
	if err := Set(name); err != nil {
		panic(err)
	}
}

// Location returns the reporting time zone.
func Location() *time.Location { return loc.Load() }

// Day returns the calendar day (YYYY-MM-DD) of a millisecond timestamp in the reporting zone.
func Day(ms int64) string { return time.UnixMilli(ms).In(Location()).Format("2006-01-02") }

// Today returns the current calendar day in the reporting zone.
func Today() string { return time.Now().In(Location()).Format("2006-01-02") }

// StartOfDay returns midnight of t's calendar day in the reporting zone.
func StartOfDay(t time.Time) time.Time {
	t = t.In(Location())
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Location())
}

// ParseDay parses YYYY-MM-DD as midnight in the reporting zone.
func ParseDay(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, Location())
}

func toInt64(v driver.Value) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		return int64(x), true
	}
	return 0, false
}

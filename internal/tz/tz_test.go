package tz

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSQLFunctionsFollowZoneAndDST(t *testing.T) {
	MustSet("America/New_York")
	defer MustSet("UTC")
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// 2026-03-08 is the US daylight-saving switch: 03:30 UTC is still 22:30 on the 7th (EST),
	// while 04:30 UTC on 2026-03-09 is 00:30 on the 9th (EDT).
	cases := map[int64]string{
		time.Date(2026, 3, 8, 3, 30, 0, 0, time.UTC).UnixMilli(): "2026-03-07",
		time.Date(2026, 3, 9, 4, 30, 0, 0, time.UTC).UnixMilli(): "2026-03-09",
	}
	for ms, want := range cases {
		var got string
		if err := db.QueryRow(`SELECT local_date(?)`, ms).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("local_date(%d) = %s, want %s", ms, got, want)
		}
	}
	var today string
	db.QueryRow(`SELECT local_today()`).Scan(&today)
	if today != Today() {
		t.Errorf("local_today() = %s, want %s", today, Today())
	}
	if Set("Nowhere/Land") == nil {
		t.Error("unknown zone accepted")
	}
}

package store

import (
	"path/filepath"
	"testing"
)

// TestFlight data marked as production after AppTransaction failed: moved to sandbox based on the released list, derived tables follow
func TestReclassifyUnreleased(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	released := map[string]string{"app_version": "2.4.4", "build": "6"}
	testflight := map[string]string{"app_version": "2.4.5", "build": "4"}
	batches := []*Batch{
		// Real user (released build): untouched
		{App: "pawprint", InstallID: "real", Env: "production", Context: released, Events: []AnalyticsEvent{
			{ID: "r1", Name: "session.started", TS: ms("2026-09-24", 9), Session: "s"},
		}},
		// TestFlight mislabeled as production: two days of data, including one purchase attempt
		{App: "pawprint", InstallID: "tf", Env: "production", Context: testflight, Events: []AnalyticsEvent{
			{ID: "t1", Name: "session.started", TS: ms("2026-09-23", 13), Session: "a"},
			{ID: "t2", Name: "session.ended", TS: ms("2026-09-23", 14), Session: "a", Params: map[string]string{"duration_s": "60"}},
			{ID: "t3", Name: "session.started", TS: ms("2026-09-24", 13), Session: "b"},
			{ID: "t4", Name: "purchase.started", TS: ms("2026-09-24", 13), Session: "b",
				Params: map[string]string{"token": "ABCDEF00-0000-0000-0000-000000000009", "context": "record_limit", "product": "pro"}},
		}},
		// Other data from the same device that was already sandbox (a sandbox row exists for that day): must merge with the reclassified rows
		{App: "pawprint", InstallID: "tf", Env: "sandbox", Context: testflight, Events: []AnalyticsEvent{
			{ID: "t5", Name: "screen.viewed", TS: ms("2026-09-24", 12), Session: "b"},
		}},
	}
	if _, err := s.WriteBatches(batches); err != nil {
		t.Fatal(err)
	}

	// An empty list does nothing
	if n, err := s.ReclassifyUnreleased("pawprint", nil); err != nil || n != 0 {
		t.Fatalf("empty list: n=%d err=%v", n, err)
	}
	n, err := s.ReclassifyUnreleased("pawprint", []string{"2.4.4|6", "2.4.3|4"})
	if err != nil || n != 4 {
		t.Fatalf("reclassified %d, err %v", n, err)
	}
	// Idempotent
	if n, _ := s.ReclassifyUnreleased("pawprint", []string{"2.4.4|6"}); n != 0 {
		t.Fatalf("second run reclassified %d", n)
	}

	count := func(q string, args ...any) int {
		t.Helper()
		var c int
		if err := s.db.QueryRow(q, args...).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := count(`SELECT COUNT(*) FROM events WHERE install_id = 'tf' AND env = 'production'`); c != 0 {
		t.Errorf("tf production events = %d", c)
	}
	if c := count(`SELECT COUNT(*) FROM events WHERE install_id = 'real' AND env = 'production'`); c != 1 {
		t.Errorf("real user touched: %d", c)
	}
	if c := count(`SELECT COUNT(*) FROM installs WHERE install_id = 'tf' AND env = 'sandbox'`); c != 1 {
		t.Errorf("install env not moved")
	}
	if c := count(`SELECT COUNT(*) FROM purchase_attempts WHERE install_id = 'tf' AND env = 'sandbox'`); c != 1 {
		t.Errorf("purchase attempt env not moved")
	}
	// Daily active: one row for 9/23 (1 session, 60 seconds, 2 events); 9/24 merged into one row (1 session, 3 events); no production rows left
	if c := count(`SELECT COUNT(*) FROM daily_active WHERE install_id = 'tf' AND env = 'production'`); c != 0 {
		t.Errorf("daily_active production rows = %d", c)
	}
	var sessions, dur, evs int
	s.db.QueryRow(`SELECT sessions, duration_s, events FROM daily_active WHERE install_id = 'tf' AND day = '2026-09-23'`).Scan(&sessions, &dur, &evs)
	if sessions != 1 || dur != 60 || evs != 2 {
		t.Errorf("9/23 = %d %d %d", sessions, dur, evs)
	}
	s.db.QueryRow(`SELECT sessions, events FROM daily_active WHERE install_id = 'tf' AND day = '2026-09-24' AND env = 'sandbox'`).Scan(&sessions, &evs)
	if sessions != 1 || evs != 3 {
		t.Errorf("9/24 = %d sessions %d events", sessions, evs)
	}
}

// A TestFlight build on a development device fell back to production (and the build was released): installs with sandbox data are moved to sandbox entirely
func TestReclassifyMixedInstalls(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := map[string]string{"app_version": "1.4.8", "build": "5"}
	batches := []*Batch{
		{App: "ledgerly", InstallID: "real", Env: "production", Context: ctx, Events: []AnalyticsEvent{
			{ID: "r1", Name: "session.started", TS: ms("2026-09-25", 9), Session: "s"},
		}},
		{App: "ledgerly", InstallID: "dev", Env: "production", Context: ctx, Events: []AnalyticsEvent{
			{ID: "d1", Name: "session.started", TS: ms("2026-09-25", 10), Session: "a"},
			{ID: "d2", Name: "screen.viewed", TS: ms("2026-09-26", 10), Session: "a"},
		}},
		// Data of the same install_id under another app is unaffected
		{App: "trailmark", InstallID: "dev", Env: "production", Context: ctx, Events: []AnalyticsEvent{
			{ID: "m1", Name: "session.started", TS: ms("2026-09-25", 10), Session: "m"},
		}},
	}
	if _, err := s.WriteBatches(batches); err != nil {
		t.Fatal(err)
	}
	// Sandbox data that only shows up later (no sandbox history at write time, so the write-time guard cannot catch it)
	if _, err := s.db.Exec(`INSERT INTO events (event_id, app, env, install_id, name, ts, day, received_at, params)
		VALUES ('d3', 'ledgerly', 'sandbox', 'dev', 'session.started', ?, '2026-09-27', 0, '{}')`, ms("2026-09-27", 10)); err != nil {
		t.Fatal(err)
	}

	n, err := s.ReclassifyMixedInstalls("ledgerly")
	if err != nil || n != 2 {
		t.Fatalf("reclassified %d, err %v", n, err)
	}
	if n, _ := s.ReclassifyMixedInstalls("ledgerly"); n != 0 {
		t.Fatalf("second run reclassified %d", n)
	}
	count := func(q string) int {
		t.Helper()
		var c int
		if err := s.db.QueryRow(q).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := count(`SELECT COUNT(*) FROM daily_active WHERE app = 'ledgerly' AND install_id = 'dev' AND env = 'production'`); c != 0 {
		t.Errorf("dev daily_active production rows = %d", c)
	}
	if c := count(`SELECT COUNT(*) FROM installs WHERE app = 'ledgerly' AND install_id = 'dev' AND env = 'sandbox'`); c != 1 {
		t.Errorf("dev install env not moved")
	}
	if c := count(`SELECT COUNT(*) FROM events WHERE install_id = 'real' AND env = 'production'`); c != 1 {
		t.Errorf("real user touched")
	}
	if c := count(`SELECT COUNT(*) FROM events WHERE app = 'trailmark' AND env = 'production'`); c != 1 {
		t.Errorf("other app touched")
	}
}

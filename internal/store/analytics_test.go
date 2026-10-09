package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

func TestMain(m *testing.M) {
	tz.MustSet("Asia/Shanghai")
	os.Exit(m.Run())
}

// Early databases are v0 (no user_version, only appstore_events); existing transactions must survive migration
func TestMigrateFromLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO appstore_events (notification_uuid, bundle_id, environment, notification_type, signed_at, received_at)
		VALUES ('old-1', 'com.example.pawprint', 'Sandbox', 'TEST', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n, version int
	s.db.QueryRow(`SELECT COUNT(*) FROM appstore_events WHERE notification_uuid = 'old-1' AND app_account_token = ''`).Scan(&n)
	s.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if n != 1 || version != len(migrations) {
		t.Fatalf("legacy row kept=%d version=%d", n, version)
	}
	var name string
	if err := s.ro.QueryRow(`SELECT name FROM device_models WHERE identifier = 'iPhone17,1'`).Scan(&name); err != nil || name != "iPhone 16 Pro" {
		t.Fatalf("device model = %q, %v", name, err)
	}
	// Reopening must not migrate again
	s.Close()
	if s2, err := Open(path); err != nil {
		t.Fatal(err)
	} else {
		s2.Close()
	}
}

func ms(day string, hour int) int64 {
	t, _ := time.ParseInLocation("2006-01-02", day, tz.Location())
	return t.Add(time.Duration(hour) * time.Hour).UnixMilli()
}

func TestWriteBatchesMaintainsDerivedTables(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := map[string]string{"app_version": "2.4.5", "device": "iPhone17,1", "os_version": "26.1", "region": "CN"}
	b1 := &Batch{App: "pawprint", InstallID: "u1", Env: "production", Context: ctx, Events: []AnalyticsEvent{
		{ID: "e1", Name: "app.installed", TS: ms("2026-09-20", 10), Params: map[string]string{"original_download_ts": "1600000000000"}},
		{ID: "e2", Name: "session.started", TS: ms("2026-09-20", 10), Session: "s1"},
		{ID: "e3", Name: "paywall.shown", TS: ms("2026-09-20", 11), Session: "s1", Params: map[string]string{"context": "record_limit"}},
		{ID: "e4", Name: "purchase.started", TS: ms("2026-09-20", 12), Session: "s1",
			Params: map[string]string{"context": "record_limit", "product": "pro", "token": "ABCDEF00-0000-0000-0000-000000000001"}},
	}}
	b2 := &Batch{App: "pawprint", InstallID: "u1", Env: "production", Context: ctx, Events: []AnalyticsEvent{
		{ID: "e5", Name: "session.ended", TS: ms("2026-09-20", 13), Session: "s1", Params: map[string]string{"duration_s": "120"}},
		{ID: "e6", Name: "session.started", TS: ms("2026-09-22", 9), Session: "s2"},
	}}
	if n, err := s.WriteBatches([]*Batch{b1, b2}); err != nil || n != 6 {
		t.Fatalf("written=%d err=%v", n, err)
	}
	// The SDK retries the same batch: nothing is counted twice
	if n, _ := s.WriteBatches([]*Batch{b2}); n != 0 {
		t.Fatalf("duplicate batch wrote %d events", n)
	}

	var sessions, duration, events int
	s.db.QueryRow(`SELECT sessions, duration_s, events FROM daily_active WHERE install_id = 'u1' AND day = '2026-09-20'`).
		Scan(&sessions, &duration, &events)
	if sessions != 1 || duration != 120 || events != 5 {
		t.Fatalf("daily_active = %d %d %d", sessions, duration, events)
	}
	var firstSeen, acquired int64
	var isNew int
	s.db.QueryRow(`SELECT first_seen, acquired_at FROM installs WHERE install_id = 'u1'`).Scan(&firstSeen, &acquired)
	s.db.QueryRow(`SELECT is_new FROM v_user_summary WHERE install_id = 'u1'`).Scan(&isNew)
	if firstSeen != ms("2026-09-20", 10) || acquired != 1600000000000 || isNew != 0 {
		t.Fatalf("installs first_seen=%d acquired=%d is_new=%d", firstSeen, acquired, isNew)
	}

	// Attribution: token is case-insensitive
	a, err := s.FindAttribution("abcdef00-0000-0000-0000-000000000001")
	if err != nil || a == nil {
		t.Fatalf("attribution = %v, %v", a, err)
	}
	if a.Context != "record_limit" || a.PaywallViews != 1 || a.Device != "iPhone 16 Pro" || a.InstallID != "u1" {
		t.Fatalf("attribution = %+v", a)
	}
	if a, _ := s.FindAttribution("missing"); a != nil {
		t.Fatal("unexpected attribution")
	}

	// Purging only deletes raw events
	if n, _ := s.PurgeEvents(time.UnixMilli(ms("2026-09-21", 0))); n != 5 {
		t.Fatalf("purged %d", n)
	}
	var left int
	s.db.QueryRow(`SELECT COUNT(*) FROM daily_active`).Scan(&left)
	if left != 2 {
		t.Fatalf("daily_active rows = %d", left)
	}
}

func TestWriterFlushesOnClose(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := NewWriter(s)
	w.Enqueue(&Batch{App: "a", InstallID: "u", Env: "xcode", Context: map[string]string{},
		Events: []AnalyticsEvent{{ID: "x", Name: "session.started", TS: ms("2026-09-20", 1)}}})
	w.Close()
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n)
	if n != 1 {
		t.Fatalf("events = %d", n)
	}
}

// SDK 0.1.2: an install without an original download time but with prior_usage=1 is not truly new; only installs with neither are new
func TestPriorUsageMarksExistingUser(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ts := time.Date(2026, 9, 24, 10, 0, 0, 0, tz.Location()).UnixMilli()
	batch := func(id string, params map[string]string) *Batch {
		return &Batch{App: "pawprint", InstallID: id, Env: "production", Context: map[string]string{"app_version": "2.4.5"},
			Events: []AnalyticsEvent{{ID: id + "-1", Name: "app.installed", TS: ts, Params: params}}}
	}
	if _, err := s.WriteBatches([]*Batch{
		batch("old", map[string]string{"prior_usage": "1", "at_status": "error: x"}),
		batch("new", map[string]string{"at_status": "error: x"}),
	}); err != nil {
		t.Fatal(err)
	}
	isNew := func(id string) int {
		var v int
		if err := s.ro.QueryRow(`SELECT is_new FROM v_user_summary WHERE install_id = ?`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if isNew("old") != 0 || isNew("new") != 1 {
		t.Errorf("is_new old=%d new=%d, want 0 / 1", isNew("old"), isNew("new"))
	}
}

// An install that already has sandbox data later reports production: it is stored as sandbox
func TestWriteBatchesKeepsDevInstallOutOfProduction(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := map[string]string{"app_version": "1.4.1", "build": "5"}
	if _, err := s.WriteBatches([]*Batch{
		{App: "trailmark", InstallID: "dev", Env: "sandbox", Context: ctx, Events: []AnalyticsEvent{
			{ID: "a1", Name: "session.started", TS: ms("2026-09-25", 15), Session: "s"},
		}},
		{App: "trailmark", InstallID: "dev", Env: "production", Context: ctx, Events: []AnalyticsEvent{
			{ID: "a2", Name: "screen.viewed", TS: ms("2026-09-25", 16), Session: "s"},
		}},
		{App: "trailmark", InstallID: "real", Env: "production", Context: ctx, Events: []AnalyticsEvent{
			{ID: "b1", Name: "session.started", TS: ms("2026-09-25", 16), Session: "r"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	var prod int
	s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE env = 'production'`).Scan(&prod)
	if prod != 1 {
		t.Fatalf("production events = %d, want only the real user's", prod)
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM daily_active WHERE install_id = 'dev' AND env = 'production'`).Scan(&prod)
	if prod != 0 {
		t.Fatalf("dev daily_active production rows = %d", prod)
	}
}

// app.installed had no original download time, so the install counted as new; a later app.acquired corrects it to an existing user
func TestAppAcquiredBackfillsAcquisition(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := map[string]string{"app_version": "2.4.6"}
	isNew := func() int {
		t.Helper()
		var v int
		if err := s.db.QueryRow(`SELECT is_new FROM v_user_summary WHERE install_id = 'u'`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if _, err := s.WriteBatches([]*Batch{{App: "pawprint", InstallID: "u", Env: "production", Context: ctx,
		Events: []AnalyticsEvent{
			{ID: "i", Name: "app.installed", TS: ms("2026-09-28", 18), Params: map[string]string{"at_status": "error: unknown"}},
		}}}); err != nil {
		t.Fatal(err)
	}
	if isNew() != 1 {
		t.Fatal("without download time the install should count as new")
	}
	if _, err := s.WriteBatches([]*Batch{{App: "pawprint", InstallID: "u", Env: "production", Context: ctx,
		Events: []AnalyticsEvent{
			{ID: "q", Name: "app.acquired", TS: ms("2026-09-29", 9),
				Params: map[string]string{"original_download_ts": "1774000000000", "at_status": "ok"}},
		}}}); err != nil {
		t.Fatal(err)
	}
	if isNew() != 0 {
		t.Fatal("app.acquired with an earlier download should mark the install as not new")
	}
}

package report

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

func TestAlerts(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	now := time.Now().In(tz.Location())
	ev := func(id, name string, params map[string]string) store.AnalyticsEvent {
		return store.AnalyticsEvent{ID: id, Name: name, TS: now.UnixMilli(), Session: "s", Params: params}
	}
	batch := func(install string, received time.Time, events ...store.AnalyticsEvent) *store.Batch {
		return &store.Batch{App: "pawprint", InstallID: install, Env: "production", Context: map[string]string{"app_version": "2.4.5"},
			ReceivedAt: received.UnixMilli(), Events: events}
	}
	fail := func(id, reason string) store.AnalyticsEvent {
		return ev(id, "purchase.failed", map[string]string{"reason": reason, "product": "com.example.pawprint.pro.yearly", "context": "pro_banner"})
	}
	errEv := func(id, errID string) store.AnalyticsEvent { return ev(id, "error", map[string]string{"id": errID}) }
	if _, err := s.WriteBatches([]*store.Batch{
		batch("u1", now, fail("p1", "product_missing"), fail("p2", "product_missing"), fail("p3", "user_cancelled"), fail("p4", "pending"),
			errEv("e1", "restore"), errEv("e2", "ai.timeout"), errEv("e3", "brand.new")),
		batch("u2", now, fail("p5", "product_missing"), errEv("e4", "ai.timeout"), errEv("e6", "sync.slow")),
		// ai.timeout and sync.slow were already seen yesterday (fewer than 3 users, so not a spike)
		batch("u0", now.Add(-30*time.Hour), errEv("old", "ai.timeout"), errEv("old2", "sync.slow")),
	}); err != nil {
		t.Fatal(err)
	}

	alerts, err := checkAlerts(context.Background(), s.ReadOnly(), testConfig, now)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, a := range alerts {
		keys = append(keys, a.rule+":"+a.key)
	}
	sort.Strings(keys)
	want := "error:pawprint|brand.new,error:pawprint|restore,purchase:pawprint|product_missing"
	if strings.Join(keys, ",") != want {
		t.Fatalf("alerts = %v, want %s", keys, want)
	}
	for _, a := range alerts {
		if a.rule == "purchase" && a.view.Subject != "Purchase failed · Pawprint · product_missing · 2 users, 3 times" {
			t.Errorf("purchase subject = %q", a.view.Subject)
		}
	}
	inZH(func() {
		zh, err := checkAlerts(context.Background(), s.ReadOnly(), testConfig, now)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range zh {
			if a.rule == "purchase" && a.view.Subject != "购买失败 · Pawprint · product_missing · 2 人 3 次" {
				t.Errorf("zh purchase subject = %q", a.view.Subject)
			}
		}
	})

	// one more user reports ai.timeout: 3 users within the hour, a spike
	if _, err := s.WriteBatches([]*store.Batch{batch("u3", now, errEv("e5", "ai.timeout"))}); err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{}
	al := &Alerter{DB: s.ReadOnly(), Ledger: s, Sender: sender, Config: testConfig, Now: func() time.Time { return now }}
	al.Tick()
	al.Tick() // not sent twice on the same day
	if len(sender.subjects) != 4 {
		t.Fatalf("subjects = %v", sender.subjects)
	}
	if !strings.Contains(strings.Join(sender.subjects, "\n"), "ai.timeout · 3 users") {
		t.Errorf("spike alert missing: %v", sender.subjects)
	}
}

func TestStaleAndSilentAlerts(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	now := time.Now().In(tz.Location())
	day := now.Format("2006-01-02")
	if err := s.ReplaceSales("DAILY", addDays(day, -4), "empty", nil); err != nil {
		t.Fatal(err)
	}
	// events 3 days ago, none since
	if _, err := s.WriteBatches([]*store.Batch{{App: "pawprint", InstallID: "u", Env: "production", ReceivedAt: now.UnixMilli(),
		Events: []store.AnalyticsEvent{{ID: "x", Name: "session.started", TS: now.Add(-72 * time.Hour).UnixMilli(), Session: "s"}}}}); err != nil {
		t.Fatal(err)
	}
	// received_at is the write time, so simulate the gap with a "now" 3 days later
	later := now.Add(72 * time.Hour)
	alerts, err := checkAlerts(context.Background(), s.ReadOnly(), testConfig, later)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, a := range alerts {
		keys = append(keys, a.rule)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "asc_stale,silent" {
		t.Fatalf("alerts = %v", keys)
	}
	// report up to date and events just received: no alerts
	if err := s.ReplaceSales("DAILY", addDays(day, -2), "ok", nil); err != nil {
		t.Fatal(err)
	}
	if alerts, _ := checkAlerts(context.Background(), s.ReadOnly(), testConfig, now); len(alerts) != 0 {
		t.Fatalf("unexpected alerts: %v", alerts)
	}
}

package metrics

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

func TestTier(t *testing.T) {
	for days, want := range map[int]string{0: "dormant", 1: "once", 2: "light", 4: "light", 5: "medium", 14: "medium", 15: "heavy", 28: "heavy"} {
		if got := Tier(days); got != want {
			t.Errorf("Tier(%d) = %s, want %s", days, got, want)
		}
	}
}

func TestOverview(t *testing.T) {
	s := seed(t)
	r := run(t, s, "overview", "2026-09-17", "2026-09-23")
	kpi := map[string][]any{}
	for _, row := range table(t, r, "kpis").Rows {
		kpi[fmt.Sprint(row[0])] = row
	}
	// 9/17-9/23: A active 9/20-23, C active every day -> 2 active users; 0 truly new (A and B both installed on 9/1)
	if got := fmt.Sprint(kpi["active_users"][1]); got != "2" {
		t.Errorf("active_users = %s, want 2", got)
	}
	if got := fmt.Sprint(kpi["new_users"][1]); got != "0" {
		t.Errorf("new_users = %s, want 0", got)
	}
	if got := fmt.Sprint(kpi["net"][1]); got != "18" {
		t.Errorf("net = %s, want 18", got)
	}
	// Previous period 9/10-9/16 only has C (9/15, 9/16)
	if got := fmt.Sprint(kpi["active_users"][2]); got != "1" {
		t.Errorf("previous active_users = %s, want 1", got)
	}
	if n := len(table(t, r, "daily").Rows); n != 7 {
		t.Errorf("daily rows = %d, want 7", n)
	}
}

func TestUsers(t *testing.T) {
	s := seed(t)
	r := run(t, s, "users", "2026-09-01", "2026-09-23")
	sum := table(t, r, "summary").Rows[0]
	if fmt.Sprint(sum[0]) != "3" || fmt.Sprint(sum[4]) != "1" {
		t.Fatalf("summary = %v, want total 3, paid 1", sum)
	}
	tiers := map[string]string{}
	for _, row := range table(t, r, "users").Rows {
		tiers[fmt.Sprint(row[0])] = fmt.Sprint(row[1])
	}
	// Last 28 days up to 9/23: A 7 days (medium), B 1 day (once), C 9 days (medium)
	if tiers["A"] != "medium" || tiers["B"] != "once" || tiers["C"] != "medium" {
		t.Errorf("tiers = %v", tiers)
	}

	res, err := Run(context.Background(), s.ReadOnly(), "users", Params{App: "pawprint", Env: "production",
		From: "2026-09-01", To: "2026-09-23", Today: "2026-09-23", Args: map[string]string{"paid": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if rows := table(t, res, "users").Rows; len(rows) != 1 || rows[0][0] != "A" {
		t.Errorf("paid filter = %v, want only A", rows)
	}
}

func TestUsersPro(t *testing.T) {
	s := seed(t)
	snap := func(id, install, day string, hour int, pro string) *store.Batch {
		return &store.Batch{App: "pawprint", InstallID: install, Env: "production",
			Context: map[string]string{"device": "iPhone17,1", "os_version": "26.1", "app_version": "2.4.5", "region": "CN"},
			Events:  []store.AnalyticsEvent{{ID: id, Name: "user.snapshot", TS: ms(day, hour), Session: day, Params: map[string]string{"is_pro": pro}}}}
	}
	// A: snapshot says free on the morning of 9/20, then buys at noon -> Pro through the confirmed purchase.
	// B: was Pro, a later snapshot the same day says free (expired) -> not Pro.
	// C: legacy member who bought before the SDK, no purchase on record -> Pro from the snapshot alone.
	if _, err := s.WriteBatches([]*store.Batch{
		snap("sa", "A", "2026-09-20", 10, "false"),
		snap("sb1", "B", "2026-09-01", 9, "true"),
		snap("sb2", "B", "2026-09-01", 10, "false"),
		snap("sc", "C", "2026-09-22", 9, "true"),
	}); err != nil {
		t.Fatal(err)
	}

	r := run(t, s, "users", "2026-09-01", "2026-09-23")
	if sum := table(t, r, "summary").Rows[0]; fmt.Sprint(sum[4], sum[6]) != "1 2" {
		t.Fatalf("summary = %v, want paid 1, pro 2", sum)
	}
	res, err := Run(context.Background(), s.ReadOnly(), "users", Params{App: "pawprint", Env: "production",
		From: "2026-09-01", To: "2026-09-23", Today: "2026-09-23", Args: map[string]string{"pro": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, row := range table(t, res, "users").Rows {
		ids = append(ids, fmt.Sprint(row[0]))
	}
	slices.Sort(ids)
	if fmt.Sprint(ids) != "[A C]" {
		t.Errorf("pro filter = %v, want [A C]", ids)
	}

	// An App Store refund of A's purchase ends the membership even if A never opens the app again.
	if _, err := s.Record(store.Event{NotificationUUID: "n3", BundleID: "com.example.pawprint", Environment: "Production",
		NotificationType: "REFUND", ProductID: "pro", Storefront: "CHN", Currency: "CNY", PriceMilli: 18000,
		RevenueSign: -1, SignedAt: ms("2026-09-22", 12), AppAccountToken: "T1"}); err != nil {
		t.Fatal(err)
	}
	if sum := table(t, run(t, s, "users", "2026-09-01", "2026-09-23"), "summary").Rows[0]; fmt.Sprint(sum[6]) != "1" {
		t.Errorf("after A's refund: summary = %v, want pro 1 (C only)", sum)
	}

	// A later snapshot after the purchase wins over the purchase.
	s2 := seed(t)
	if _, err := s2.WriteBatches([]*store.Batch{snap("sa2", "A", "2026-09-23", 9, "false")}); err != nil {
		t.Fatal(err)
	}
	if sum := table(t, run(t, s2, "users", "2026-09-01", "2026-09-23"), "summary").Rows[0]; fmt.Sprint(sum[4], sum[6]) != "1 0" {
		t.Errorf("after A's free snapshot: summary = %v, want paid 1, pro 0", sum)
	}
}

func TestEventTrend(t *testing.T) {
	s := seed(t)
	res, err := Run(context.Background(), s.ReadOnly(), "event_trend", Params{App: "pawprint", Env: "production",
		From: "2026-09-14", To: "2026-09-23", Today: "2026-09-23",
		Args: map[string]string{"event": "screen.viewed", "group_by": "module", "granularity": "week"}})
	if err != nil {
		t.Fatal(err)
	}
	buckets := table(t, res, "buckets").Rows
	if len(buckets) != 2 || buckets[0][0] != "2026-09-14" || buckets[1][0] != "2026-09-21" {
		t.Errorf("week buckets = %v, want Mondays 9/14 and 9/21", buckets)
	}
	br := map[string]string{}
	for _, row := range table(t, res, "breakdown").Rows {
		br[fmt.Sprint(row[0])] = fmt.Sprint(row[1])
	}
	if br["pet"] != "2" || br["timeline"] != "1" {
		t.Errorf("breakdown = %v", br)
	}

	if _, err := Run(context.Background(), s.ReadOnly(), "event_trend", Params{App: "pawprint", Env: "production",
		From: "2026-09-14", To: "2026-09-23", Args: map[string]string{"group_by": "x') OR 1=1 --"}}); err == nil {
		t.Error("invalid group_by accepted")
	}
}

package metrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

func TestMain(m *testing.M) {
	tz.MustSet("Asia/Shanghai")
	os.Exit(m.Run())
}

func ms(day string, hour int) int64 {
	t, _ := time.ParseInLocation("2006-01-02", day, tz.Location())
	return t.Add(time.Duration(hour) * time.Hour).UnixMilli()
}

// Synthetic data (today = 2026-09-23):
//   - A: new install on 9/1; active 9/1, 9/2, 9/8, then absent for 11 days, resurrected 9/20-9/23; paid on 9/20 after hitting the record limit
//   - B: new install on 9/1, came only once
//   - C: existing user (downloaded back in 2025), upgraded to a version with the SDK on 9/15, active every day since
func seed(t *testing.T) *store.Store {
	s, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	n := 0
	ev := func(name, day string, hour int, params map[string]string) store.AnalyticsEvent {
		n++
		return store.AnalyticsEvent{ID: fmt.Sprint("e", n), Name: name, TS: ms(day, hour), Session: day, Params: params}
	}
	batch := func(install string, events ...store.AnalyticsEvent) *store.Batch {
		return &store.Batch{App: "pawprint", InstallID: install, Env: "production",
			Context: map[string]string{"device": "iPhone17,1", "os_version": "26.1", "app_version": "2.4.5", "region": "CN"}, Events: events}
	}
	var batches []*store.Batch

	a := []store.AnalyticsEvent{ev("app.installed", "2026-09-01", 9, map[string]string{"original_download_ts": fmt.Sprint(ms("2026-09-01", 9))})}
	for _, d := range []string{"2026-09-01", "2026-09-02", "2026-09-08", "2026-09-20", "2026-09-21", "2026-09-22", "2026-09-23"} {
		a = append(a, ev("session.started", d, 10, nil))
	}
	a = append(a,
		ev("screen.viewed", "2026-09-20", 11, map[string]string{"screen": "pet_profile", "module": "pet"}),
		ev("screen.left", "2026-09-20", 11, map[string]string{"screen": "pet_profile", "module": "pet", "duration_s": "30"}),
		ev("screen.viewed", "2026-09-21", 11, map[string]string{"screen": "pet_profile", "module": "pet"}),
		ev("screen.left", "2026-09-21", 11, map[string]string{"screen": "pet_profile", "module": "pet", "duration_s": "10"}),
		ev("ai.used", "2026-09-20", 11, map[string]string{"module": "ai", "feature": "plan", "result": "success"}),
		ev("ai.used", "2026-09-21", 11, map[string]string{"module": "ai", "feature": "plan", "result": "success"}),
		ev("ai.used", "2026-09-21", 12, map[string]string{"module": "ai", "feature": "plan", "result": "fail"}),
		ev("ocr.used", "2026-09-21", 12, map[string]string{"module": "trade", "source": "trade", "result": "success"}),
		ev("paywall.shown", "2026-09-20", 12, map[string]string{"context": "record_limit"}),
		ev("paywall.cta_tap", "2026-09-20", 12, map[string]string{"context": "record_limit"}),
		ev("purchase.started", "2026-09-20", 12, map[string]string{"context": "record_limit", "product": "pro", "token": "t1"}),
		ev("purchase.success", "2026-09-20", 12, map[string]string{"context": "record_limit", "product": "pro"}),
	)
	batches = append(batches, batch("A", a...))
	batches = append(batches, batch("B",
		ev("app.installed", "2026-09-01", 9, map[string]string{"original_download_ts": fmt.Sprint(ms("2026-09-01", 9))}),
		ev("session.started", "2026-09-01", 9, nil)))

	c := []store.AnalyticsEvent{ev("app.installed", "2026-09-15", 9, map[string]string{"original_download_ts": fmt.Sprint(ms("2025-03-01", 9))})}
	for d := 15; d <= 23; d++ {
		c = append(c, ev("session.started", fmt.Sprintf("2026-09-%02d", d), 9, nil))
	}
	c = append(c,
		ev("screen.viewed", "2026-09-16", 11, map[string]string{"screen": "timeline", "module": "timeline"}),
		ev("paywall.shown", "2026-09-16", 12, map[string]string{"context": "settings_banner"}))
	batches = append(batches, batch("C", c...))

	if _, err := s.WriteBatches(batches); err != nil {
		t.Fatal(err)
	}
	amount := 18.0
	if _, err := s.Record(store.Event{NotificationUUID: "n1", BundleID: "com.example.pawprint", Environment: "Production",
		NotificationType: "ONE_TIME_CHARGE", ProductID: "pro", Storefront: "CHN", Currency: "CNY", PriceMilli: 18000,
		RevenueSign: 1, Amount: &amount, SignedAt: ms("2026-09-20", 12), AppAccountToken: "T1"}); err != nil {
		t.Fatal(err)
	}
	// Free-trial start (price 0): listed separately as trials, not counted as a sale
	zero := 0.0
	if _, err := s.Record(store.Event{NotificationUUID: "n2", BundleID: "com.example.pawprint", Environment: "Production",
		NotificationType: "SUBSCRIBED", ProductID: "pro.monthly", Storefront: "CHN", Currency: "CNY", PriceMilli: 0,
		RevenueSign: 1, Amount: &zero, SignedAt: ms("2026-09-21", 12)}); err != nil {
		t.Fatal(err)
	}
	return s
}

func run(t *testing.T, s *store.Store, name, from, to string) *Result {
	t.Helper()
	res, err := Run(context.Background(), s.ReadOnly(), name, Params{
		App: "pawprint", Bundle: "com.example.pawprint", Env: "production", From: from, To: to, Today: "2026-09-23"})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func table(t *testing.T, r *Result, name string) Table {
	t.Helper()
	for _, tb := range r.Tables {
		if tb.Name == name {
			return tb
		}
	}
	t.Fatalf("%s: no table %q", r.Metric, name)
	return Table{}
}

func find(tb Table, key any) []any {
	for _, r := range tb.Rows {
		if fmt.Sprint(r[0]) == fmt.Sprint(key) {
			return r
		}
	}
	return nil
}

func TestRetention(t *testing.T) {
	s := seed(t)
	cohorts := table(t, run(t, s, "retention", "2026-09-01", "2026-09-23"), "cohorts")
	row := find(cohorts, "2026-09-01")
	// cohort_day, new_users, d1, d3, d7, d14, d30, within_7d, within_30d
	want := []any{"2026-09-01", 2, 50.0, 0.0, 50.0, 0.0, nil, 50.0, nil}
	if fmt.Sprint(row) != fmt.Sprint(want) {
		t.Fatalf("cohort row = %v, want %v", row, want)
	}
	// Existing user C is not in any retention cohort
	if find(cohorts, "2026-09-15") != nil {
		t.Fatal("legacy user counted as new")
	}
}

func TestActivityDays(t *testing.T) {
	s := seed(t)
	r := run(t, s, "activity_days", "2026-09-23", "2026-09-23")
	l7 := table(t, r, "l7")
	if find(l7, 4)[1] != 1 || find(l7, 7)[1] != 1 || find(l7, 1)[1] != 0 {
		t.Fatalf("l7 = %v", l7.Rows)
	}
	l28 := table(t, r, "l28")
	if find(l28, "1")[1] != 1 || find(l28, "4-7")[1] != 1 || find(l28, "8-14")[1] != 1 {
		t.Fatalf("l28 = %v", l28.Rows)
	}
}

func TestReturningAndActive(t *testing.T) {
	s := seed(t)
	r := run(t, s, "returning", "2026-09-20", "2026-09-23")
	day := find(table(t, r, "daily"), "2026-09-20") // day, active, new, returning, resurrected
	if fmt.Sprint(day) != "[2026-09-20 2 0 2 1]" {
		t.Fatalf("returning 9/20 = %v", day)
	}
	sum := table(t, r, "summary").Rows[0] // users_ever, resurrected, at_risk, churned
	if fmt.Sprint(sum) != "[3 1 0 1]" {
		t.Fatalf("summary = %v", sum)
	}

	a := run(t, s, "active_users", "2026-09-23", "2026-09-23")
	d := table(t, a, "daily").Rows[0] // day, dau, wau, mau, new, stickiness
	if fmt.Sprint(d[:5]) != "[2026-09-23 2 2 3 0]" {
		t.Fatalf("active 9/23 = %v", d)
	}
}

func TestModuleFunnelRevenueDistribution(t *testing.T) {
	s := seed(t)
	mod := table(t, run(t, s, "module_usage", "2026-09-01", "2026-09-23"), "modules")
	pet := find(mod, "pet") // module, users, coverage, views, views_per_user, avg_duration
	if fmt.Sprint(pet) != "[pet 1 33.3 2 2 20]" {
		t.Fatalf("pet module = %v", pet)
	}
	// module, event, feature, users, coverage, uses, uses_per_user, success_pct; screen events are excluded
	feat := table(t, run(t, s, "module_usage", "2026-09-01", "2026-09-23"), "features")
	if fmt.Sprint(feat.Rows) != "[[ai ai.used plan 1 33.3 3 3 66.7] [trade ocr.used trade 1 33.3 1 1 100]]" {
		t.Fatalf("features = %v", feat.Rows)
	}

	f := table(t, run(t, s, "funnel", "2026-09-01", "2026-09-23"), "by_context")
	rl := find(f, "record_limit") // context, shown, cta, started, success, verified, verified_trials, cta_rate, success_rate
	if fmt.Sprint(rl) != "[record_limit 1 1 1 1 1 0 100 100]" {
		t.Fatalf("funnel record_limit = %v", rl)
	}

	rev := run(t, s, "revenue", "2026-09-01", "2026-09-23")
	if ctx := find(table(t, rev, "by_context"), "record_limit"); ctx == nil || fmt.Sprint(ctx[4]) != "18" { // key, sales, trials, refunds, net
		t.Fatalf("revenue by_context = %v", table(t, rev, "by_context").Rows)
	}
	var dailyNet float64 // the sum of daily net revenue must equal the summary
	for _, r := range table(t, rev, "daily").Rows {
		dailyNet += r[4].(float64)
	}
	if sum := table(t, rev, "summary").Rows[0][4]; fmt.Sprint(dailyNet) != fmt.Sprint(sum) {
		t.Fatalf("revenue daily net = %v, summary = %v", dailyNet, sum)
	}
	if sum := table(t, rev, "summary").Rows[0]; fmt.Sprint(sum[1:4]) != "[1 1 0]" { // sales, trials, refunds
		t.Fatalf("revenue summary = %v", sum)
	}
	if d := table(t, rev, "days_to_purchase").Rows; fmt.Sprint(d) != "[[8-30 1]]" {
		t.Fatalf("days_to_purchase = %v", d)
	}

	dev := table(t, run(t, s, "distribution", "2026-09-20", "2026-09-23"), "device")
	if fmt.Sprint(dev.Rows) != "[[iPhone 16 Pro 2 100]]" {
		t.Fatalf("device = %v", dev.Rows)
	}

	u, err := User(context.Background(), s.ReadOnly(), "pawprint", "A", 5)
	if err != nil || len(u.Tables[2].Rows) != 5 || len(u.Tables[1].Rows) != 7 {
		t.Fatalf("user = %+v, %v", u, err)
	}
}

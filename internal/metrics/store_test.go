package metrics

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

func TestStoreFunnel(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	eng := func(date, bundle, source string, impU, pvU int) store.FunnelRecord {
		return store.FunnelRecord{Date: date, BundleID: bundle, Source: source, Territory: "CN",
			Impressions: impU + 5, ImpressionsUnique: impU, PageViews: pvU + 1, PageViewsUnique: pvU}
	}
	dl := func(date, bundle, source string, first int) store.FunnelRecord {
		return store.FunnelRecord{Date: date, BundleID: bundle, Source: source, Territory: "CN", FirstDownloads: first}
	}
	// The instance processed on 9/21 covers 9/20-9/21; the 9/22 instance corrects 9/21 and adds 9/22: 9/21 must use the latter
	must(s.ApplyAnalyticsInstance("e1", "D", "a.app", "2026-09-21", true, []store.FunnelRecord{
		eng("2026-09-20", "a.app", "search", 100, 10), eng("2026-09-21", "a.app", "search", 999, 99)}))
	must(s.ApplyAnalyticsInstance("e2", "D", "a.app", "2026-09-22", true, []store.FunnelRecord{
		eng("2026-09-21", "a.app", "search", 200, 20), eng("2026-09-22", "a.app", "browse", 50, 5)}))
	must(s.ApplyAnalyticsInstance("d1", "L", "a.app", "2026-09-22", false, []store.FunnelRecord{
		dl("2026-09-20", "a.app", "search", 3), dl("2026-09-21", "a.app", "search", 4), dl("2026-09-22", "a.app", "browse", 1)}))
	// Previous window (9/17-9/19)
	must(s.ApplyAnalyticsInstance("e0", "D", "a.app", "2026-09-19", true, []store.FunnelRecord{eng("2026-09-18", "a.app", "search", 100, 10)}))
	must(s.ApplyAnalyticsInstance("d0", "L", "a.app", "2026-09-19", false, []store.FunnelRecord{dl("2026-09-18", "a.app", "search", 2)}))
	// Another app, only counted with scope=all
	must(s.ApplyAnalyticsInstance("e3", "D", "b.app", "2026-09-22", true, []store.FunnelRecord{eng("2026-09-22", "b.app", "search", 40, 4)}))

	ids, err := s.AnalyticsInstanceIDs()
	must(err)
	if len(ids) != 6 {
		t.Fatalf("instances = %d", len(ids))
	}

	// A 3-day filter (to is after the latest report day, so it is clamped to 9/22)
	res, err := Run(context.Background(), s.ReadOnly(), "store_funnel", Params{App: "a", Bundle: "a.app", From: "2026-09-22", To: "2026-09-24"})
	must(err)
	win := table(t, res, "window").Rows[0]
	if fmt.Sprint(win[:4]) != "[2026-09-20 2026-09-22 2026-09-17 2026-09-19]" {
		t.Errorf("window = %v", win)
	}
	if win[5] != false { // earliest data 9/18 is after the previous window start 9/17
		t.Errorf("prev_complete = %v", win[5])
	}
	apps := table(t, res, "apps").Rows
	if len(apps) != 1 {
		t.Fatalf("apps = %v", apps)
	}
	// Unique impressions 100 + 200 + 50 = 350, product page 35, first-time downloads 8; conversion 8/350 = 2.3%, product page conversion 8/35 = 22.9%
	if got := fmt.Sprint(apps[0][2], apps[0][4], apps[0][5], apps[0][8], apps[0][9]); got != "350 35 8 2.3 22.9" {
		t.Errorf("a.app = %s", got)
	}
	// Previous window: impressions 100, downloads 2, conversion 2%
	if got := fmt.Sprint(apps[0][10], apps[0][11], apps[0][12]); got != "100 2 2" {
		t.Errorf("prev = %s", got)
	}
	src := map[string][]any{}
	for _, r := range table(t, res, "sources").Rows {
		src[fmt.Sprint(r[0])] = r
	}
	if fmt.Sprint(src["search"][1], src["search"][3]) != "300 7" || fmt.Sprint(src["browse"][1], src["browse"][3]) != "50 1" {
		t.Errorf("sources = %v", src)
	}

	all, err := Run(context.Background(), s.ReadOnly(), "store_funnel", Params{App: "a", Bundle: "a.app", From: "2026-09-20", To: "2026-09-22",
		Args: map[string]string{"scope": "all"}})
	must(err)
	if n := len(table(t, all, "apps").Rows); n != 2 {
		t.Errorf("scope=all apps = %d", n)
	}
}

func TestStoreFunnelEmpty(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	res, err := Run(context.Background(), s.ReadOnly(), "store_funnel", Params{App: "a", Bundle: "a.app"})
	if err != nil {
		t.Fatal(err)
	}
	if st := table(t, res, "status").Rows[0]; st[2] != false {
		t.Errorf("status = %v", st)
	}
}

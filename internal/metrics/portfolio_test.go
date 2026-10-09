package metrics

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

func TestPortfolio(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.UpsertASCApps([]store.ASCApp{{AppleID: "1", BundleID: "a.app", SKU: "a", Name: "A"}, {AppleID: "2", BundleID: "b.app", SKU: "b", Name: "B"}})
	f := func(v float64) *float64 { return &v }
	sale := func(bundle string, units, proceeds float64) store.SalesRecord {
		return store.SalesRecord{BundleID: bundle, SKU: bundle + ".pro", ProductType: "IA1", Category: "iap", Units: units, ProceedsPerUnit: 10, Proceeds: f(proceeds)}
	}
	dl := func(bundle string, units float64) store.SalesRecord {
		return store.SalesRecord{BundleID: bundle, SKU: bundle, ProductType: "1F", Category: "download", Units: units, Proceeds: f(0)}
	}
	// September daily reports (through 9/22), August daily reports (8/1-8/22 is the same period last month),
	// July has both a monthly report and daily reports (only the monthly report should count)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.ReplaceSales("DAILY", "2026-09-05", "ok", []store.SalesRecord{sale("a.app", 2, 20), dl("a.app", 10)}))
	must(s.ReplaceSales("DAILY", "2026-09-22", "ok", []store.SalesRecord{sale("b.app", 1, 10), sale("a.app", -1, -10)}))
	must(s.ReplaceSales("DAILY", "2026-08-10", "ok", []store.SalesRecord{sale("a.app", 1, 10), dl("a.app", 5)}))
	must(s.ReplaceSales("DAILY", "2026-08-28", "ok", []store.SalesRecord{sale("b.app", 3, 30)}))
	must(s.ReplaceSales("DAILY", "2026-07-15", "ok", []store.SalesRecord{sale("a.app", 9, 90)}))
	must(s.ReplaceSales("MONTHLY", "2026-07", "ok", []store.SalesRecord{sale("a.app", 5, 50)}))

	res, err := Run(context.Background(), s.ReadOnly(), "portfolio", Params{App: "x", Env: "production", Today: "2026-09-23",
		Apps: map[string]string{"a": "a.app"}})
	if err != nil {
		t.Fatal(err)
	}
	kpi := map[string][]any{}
	for _, r := range table(t, res, "kpis").Rows {
		kpi[fmt.Sprint(r[0])] = r
	}
	check := func(key string, col int, want string) {
		t.Helper()
		if got := fmt.Sprint(kpi[key][col]); got != want {
			t.Errorf("%s[%d] = %s, want %s", key, col, got, want)
		}
	}
	check("mtd_proceeds", 1, "20")        // 20 + 10 - 10
	check("mtd_proceeds", 2, "10")        // same period last month, 8/1-8/22
	check("last_month_proceeds", 1, "40") // all of August
	check("ytd_proceeds", 1, "110")       // July monthly report only 50 + August 40 + September 20
	check("mtd_downloads", 1, "10")
	check("mtd_refunds", 1, "1")

	apps := map[string][]any{}
	for _, r := range table(t, res, "apps").Rows {
		apps[fmt.Sprint(r[0])] = r
	}
	if got := fmt.Sprint(apps["a.app"][3]); got != "10" { // A this month: 20 - 10
		t.Errorf("a.app mtd = %s", got)
	}
	if got := fmt.Sprint(apps["b.app"][6]); got != "40" { // B last 12 months: 30 + 10
		t.Errorf("b.app 12m = %s", got)
	}
	if apps["a.app"][1] != "a" || apps["b.app"][1] != "" {
		t.Errorf("analytics key mapping = %v / %v", apps["a.app"][1], apps["b.app"][1])
	}

	// Range: defaults to the last 12 months by month (July counts the monthly report only); week / month / custom use daily reports
	sum := func(range_, from, to string) (string, float64, int) {
		t.Helper()
		res, err := Run(context.Background(), s.ReadOnly(), "portfolio", Params{App: "x", Env: "production", Today: "2026-09-23",
			From: from, To: to, Args: map[string]string{"range": range_}})
		if err != nil {
			t.Fatal(err)
		}
		var total float64
		tr := table(t, res, "trend")
		for _, r := range tr.Rows {
			total += r[2].(float64)
		}
		return fmt.Sprint(table(t, res, "period").Rows[0][7]), total, len(tr.Rows)
	}
	for _, c := range []struct {
		range_, from, to, gran string
		total                  float64
	}{
		{"", "", "", "month", 110},
		{"week", "", "", "day", 0},                           // 9/16-9/22: 10 - 10
		{"month", "", "", "day", 50},                         // 8/24-9/22: 30 + 20 + 0
		{"custom", "2026-07-01", "2026-08-15", "day", 100},   // daily reports 7/15 90 + 8/10 10
		{"custom", "2026-06-01", "2026-09-22", "month", 150}, // spans more than 92 days, aggregated by month
	} {
		gran, total, _ := sum(c.range_, c.from, c.to)
		if gran != c.gran || total != c.total {
			t.Errorf("range %q %s–%s = %s %v, want %s %v", c.range_, c.from, c.to, gran, total, c.gran, c.total)
		}
	}
}

package demo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/config"
	"github.com/RedHiwiK/HiwiKInsight/internal/metrics"
	"github.com/RedHiwiK/HiwiKInsight/internal/store"
	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

func table(t *testing.T, r *metrics.Result, name string) metrics.Table {
	t.Helper()
	for _, tb := range r.Tables {
		if tb.Name == name {
			return tb
		}
	}
	t.Fatalf("%s/%s: no table %q", r.Metric, r.App, name)
	return metrics.Table{}
}

func num(v any) float64 {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case float64:
		return x
	}
	return 0
}

func col(t *testing.T, tb metrics.Table, name string) int {
	t.Helper()
	for i, c := range tb.Columns {
		if c == name {
			return i
		}
	}
	t.Fatalf("%s: no column %q in %v", tb.Name, name, tb.Columns)
	return 0
}

// cell returns a column of the first row whose first column equals key ("" = first row).
func cell(t *testing.T, tb metrics.Table, key, name string) float64 {
	t.Helper()
	i := col(t, tb, name)
	for _, row := range tb.Rows {
		if key == "" || row[0] == key {
			return num(row[i])
		}
	}
	t.Fatalf("%s: no row %q", tb.Name, key)
	return 0
}

func TestPrepareFillsEveryPage(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Prepare(dir, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Apps) != 3 || cfg.Timezone != timezone || len(cfg.Dashboard.Users) != 1 || cfg.Query.Tokens[0] != queryToken {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	tz.MustSet(cfg.Timezone)
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db := st.ReadOnly()
	ctx := context.Background()

	today := tz.Today()
	from := time.Now().In(tz.Location()).AddDate(0, 0, -29).Format("2006-01-02")
	all := map[string]string{}
	for _, a := range cfg.Apps {
		all[a.Key] = a.BundleID
	}
	run := func(name string, a config.App, args map[string]string) *metrics.Result {
		t.Helper()
		r, err := metrics.Run(ctx, db, name, metrics.Params{App: a.Key, Bundle: a.BundleID, Env: "production",
			From: from, To: today, Today: today, Apps: all, Args: args})
		if err != nil {
			t.Fatalf("%s/%s: %v", name, a.Key, err)
		}
		return r
	}

	for _, a := range cfg.Apps {
		if _, err := os.Stat(a.Catalog); err != nil {
			t.Errorf("%s: catalog: %v", a.Key, err)
		}
		ov := run("overview", a, nil)
		if len(ov.Tables) == 0 || len(ov.Tables[0].Rows) == 0 {
			t.Errorf("%s: overview is empty", a.Key)
		}

		ret := run("retention", a, nil)
		if len(table(t, ret, "cohorts").Rows) < 20 {
			t.Errorf("%s: retention has %d cohorts", a.Key, len(table(t, ret, "cohorts").Rows))
		}

		fn := table(t, run("funnel", a, nil), "by_context")
		var shown, sales float64
		for _, row := range fn.Rows {
			shown += num(row[col(t, fn, "shown_users")])
			sales += num(row[col(t, fn, "verified_sales")]) + num(row[col(t, fn, "verified_trials")])
		}
		if shown == 0 || sales == 0 {
			t.Errorf("%s: funnel shown=%v verified=%v", a.Key, shown, sales)
		}

		rev := run("revenue", a, nil)
		if net := cell(t, table(t, rev, "summary"), "", "net"); net <= 0 {
			t.Errorf("%s: revenue net = %v", a.Key, net)
		}
		if len(table(t, rev, "by_storefront").Rows) < 2 {
			t.Errorf("%s: revenue from a single storefront", a.Key)
		}

		mu := run("module_usage", a, nil)
		if len(table(t, mu, "modules").Rows) < 5 || len(table(t, mu, "features").Rows) == 0 {
			t.Errorf("%s: module_usage modules=%d features=%d", a.Key, len(table(t, mu, "modules").Rows), len(table(t, mu, "features").Rows))
		}

		er := run("errors", a, nil)
		if cell(t, table(t, er, "summary"), "", "count") == 0 {
			t.Errorf("%s: no errors", a.Key)
		}

		sf := run("store_funnel", a, nil)
		if cell(t, table(t, sf, "apps"), "", "first_downloads") <= 0 {
			t.Errorf("%s: store funnel has no downloads", a.Key)
		}

		dist := run("distribution", a, nil)
		if len(dist.Tables) == 0 || len(dist.Tables[0].Rows) < 3 {
			t.Errorf("%s: distribution too small", a.Key)
		}
	}

	// sandbox and Xcode installs exist too
	var sandbox int
	if err := db.QueryRow(`SELECT COUNT(*) FROM installs WHERE env IN ('sandbox', 'xcode')`).Scan(&sandbox); err != nil || sandbox == 0 {
		t.Errorf("no sandbox installs (%v)", err)
	}

	pf := run("portfolio", cfg.Apps[0], nil)
	if s := table(t, pf, "status"); s.Rows[0][2] != true {
		t.Fatalf("portfolio not synced: %v", s.Rows)
	}
	kpis := table(t, pf, "kpis")
	for _, k := range []string{"last_month_proceeds", "ytd_proceeds", "last_12m_proceeds", "mtd_downloads"} {
		if cell(t, kpis, k, "current") <= 0 {
			t.Errorf("portfolio %s is not positive", k)
		}
	}
	if n := len(table(t, pf, "apps").Rows); n != 3 {
		t.Errorf("portfolio has %d apps", n)
	}
	if len(table(t, pf, "countries").Rows) < 5 || len(table(t, pf, "feed").Rows) == 0 {
		t.Error("portfolio countries or feed empty")
	}
	latest := table(t, pf, "status").Rows[0][0].(string)
	want := time.Now().In(pacific).AddDate(0, 0, -reportDelay).Format("2006-01-02")
	if latest != want {
		t.Errorf("newest daily report %s, want %s", latest, want)
	}

	// a second run reuses the database
	info, _ := os.Stat(cfg.DBPath())
	if _, err := Prepare(dir, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	again, _ := os.Stat(cfg.DBPath())
	if !again.ModTime().Equal(info.ModTime()) {
		t.Error("second Prepare regenerated the database")
	}
}

func TestExampleCatalogsMatchEmbedded(t *testing.T) {
	for _, a := range apps {
		embedded, err := catalogs.ReadFile("catalogs/" + a.key + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		example, err := os.ReadFile(filepath.Join("..", "..", "examples", "catalogs", a.key+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if string(example) != string(embedded) {
			t.Errorf("examples/catalogs/%s.yaml differs from internal/demo/catalogs; copy it over", a.key)
		}
	}
}

func TestExampleConfigLoads(t *testing.T) {
	for k, v := range map[string]string{
		"HIWIKINSIGHT_ADMIN_HASH":  "$2a$10$abcdefghijklmnopqrstuuJ6Yl7y1Ov0u6Wl1WQ4yMHnE7m1QpUUa",
		"HIWIKINSIGHT_QUERY_TOKEN": "t", "SMTP_PASSWORD": "p", "ASC_KEY_ID": "k", "ASC_ISSUER_ID": "i", "ASC_VENDOR_NUMBER": "v",
	} {
		t.Setenv(k, v)
	}
	cfg, err := config.Load(filepath.Join("..", "..", "examples", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Apps) == 0 || !cfg.Mail.Enabled() || !cfg.ASC.Enabled() || len(cfg.Query.Tokens) == 0 || len(cfg.Dashboard.Users) == 0 {
		t.Errorf("example config is incomplete: %+v", cfg)
	}
}

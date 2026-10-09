package report

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/i18n"
	"github.com/RedHiwiK/HiwiKInsight/internal/store"
	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

// The fixtures are written in UTC+8 days and hours.
func TestMain(m *testing.M) {
	tz.MustSet("Asia/Shanghai")
	i18n.Set("en")
	os.Exit(m.Run())
}

// inZH runs f with the language set to Chinese.
func inZH(f func()) {
	i18n.Set("zh")
	defer i18n.Set("en")
	f()
}

func ms(day string, hour int) int64 {
	t, _ := time.ParseInLocation("2006-01-02", day, tz.Location())
	return t.Add(time.Duration(hour) * time.Hour).UnixMilli()
}

var testConfig = Config{
	Apps:         []App{{Key: "pawprint", Bundle: "com.example.pawprint", Name: "Pawprint"}},
	TypeLabel:    func(t, _ string) string { return t },
	ProductName:  func(id string) string { return id },
	ContextLabel: func(c string) string { return c },
	Alerts: AlertRules{SpikeUsers: 3, ReportStaleDays: 3, SilentHours: 48,
		PaymentErrors: map[string]bool{"purchase": true, "restore": true, "products.load": true}},
}

// Two days of data: 9/24 (first analytics day) 1 user, 9/25 2 users (1 of them a new install).
func seed(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := map[string]string{"app_version": "2.4.5"}
	ev := func(id, name, day string, params map[string]string) store.AnalyticsEvent {
		return store.AnalyticsEvent{ID: id, Name: name, TS: ms(day, 10), Session: "s", Params: params}
	}
	if _, err := s.WriteBatches([]*store.Batch{
		{App: "pawprint", InstallID: "old", Env: "production", Context: ctx, Events: []store.AnalyticsEvent{
			ev("o1", "session.started", "2026-09-24", nil),
			ev("o2", "session.started", "2026-09-25", nil),
		}},
		{App: "pawprint", InstallID: "new", Env: "production", Context: ctx, Events: []store.AnalyticsEvent{
			ev("n0", "app.installed", "2026-09-25", map[string]string{"original_download_ts": "1790290000000"}),
			ev("n1", "session.started", "2026-09-25", nil),
			ev("n2", "error", "2026-09-25", map[string]string{"id": "ai.failed", "message": "timeout"}),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDailyReport(t *testing.T) {
	s := seed(t)
	v, err := Build(context.Background(), s.ReadOnly(), testConfig, Daily, "2026-09-25", "2026-09-26")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.Subject, "Daily Report Sep 25 · ") || !strings.Contains(v.Subject, "DAU 2") {
		t.Errorf("subject = %q", v.Subject)
	}
	if v.Stats[0].Value != "2" || v.Stats[0].Delta != "+1" {
		t.Errorf("DAU stat = %+v", v.Stats[0])
	}
	var titles []string
	for _, tb := range v.Tables {
		titles = append(titles, tb.Title)
	}
	if strings.Join(titles, ",") != "Apps,New errors" {
		t.Errorf("tables = %v", titles)
	}
	html, err := v.HTML()
	if err != nil || !strings.Contains(html, "ai.failed") {
		t.Fatalf("html err=%v", err)
	}

	inZH(func() {
		zh, err := Build(context.Background(), s.ReadOnly(), testConfig, Daily, "2026-09-25", "2026-09-26")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(zh.Subject, "运营日报 9/25 · 流水 ") || !strings.Contains(zh.Subject, "DAU 2") {
			t.Errorf("zh subject = %q", zh.Subject)
		}
		if zh.Kicker != "运营日报" || zh.Title != "2026 年 9 月 25 日 周五" || zh.Tables[0].Title != "各 App" {
			t.Errorf("zh view = %q %q %q", zh.Kicker, zh.Title, zh.Tables[0].Title)
		}
	})
	if v.Title != "Friday, September 25, 2026" {
		t.Errorf("title = %q", v.Title)
	}

	// previous day is before the first analytics day: no comparison
	first, err := Build(context.Background(), s.ReadOnly(), testConfig, Daily, "2026-09-24", "2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	if first.Stats[0].Delta != "" {
		t.Errorf("first day should have no delta, got %q", first.Stats[0].Delta)
	}
}

func TestWeeklyReportAlignsToMonday(t *testing.T) {
	s := seed(t)
	v, err := Build(context.Background(), s.ReadOnly(), testConfig, Weekly, "2026-09-24", "2026-09-28") // a Thursday
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "Sep 21–27" {
		t.Errorf("title = %q", v.Title)
	}
	if v.Stats[0].Value != "2" || v.Stats[0].Delta != "" { // no data the week before: no comparison
		t.Errorf("weekly active stat = %+v", v.Stats[0])
	}
	if !strings.Contains(v.Text(), "Top 5 errors") {
		t.Error("weekly should list errors")
	}
}

type fakeSender struct{ subjects []string }

func (f *fakeSender) Send(subject, _, _ string) error {
	f.subjects = append(f.subjects, subject)
	return nil
}

func TestSchedulerSendsOncePerPeriod(t *testing.T) {
	s := seed(t)
	sender := &fakeSender{}
	now := time.Date(2026, 9, 28, 8, 0, 0, 0, tz.Location()) // Monday 08:00: not due yet
	sc := &Scheduler{DB: s.ReadOnly(), Ledger: s, Sender: sender, Config: testConfig, Hour: 9, Daily: true, Weekly: true, Now: func() time.Time { return now }}
	sc.Tick()
	if len(sender.subjects) != 0 {
		t.Fatalf("sent before 09:00: %v", sender.subjects)
	}
	now = now.Add(2 * time.Hour)
	sc.Tick()
	sc.Tick() // the same period is not sent twice
	if len(sender.subjects) != 2 || !strings.HasPrefix(sender.subjects[0], "Daily Report Sep 27") || !strings.HasPrefix(sender.subjects[1], "Weekly Report Sep 21–27") {
		t.Fatalf("subjects = %v", sender.subjects)
	}
	// nor after a restart (a new Scheduler)
	sc2 := &Scheduler{DB: s.ReadOnly(), Ledger: s, Sender: sender, Config: testConfig, Hour: 9, Daily: true, Weekly: true, Now: func() time.Time { return now }}
	sc2.Tick()
	if len(sender.subjects) != 2 {
		t.Fatalf("resent after restart: %v", sender.subjects)
	}
}

package report

// Alerts: every 5 minutes, check for problems that need prompt attention (production only) and email each hit
// to the report recipients. The same problem (rule + app + id) is sent at most once a day, recorded in
// report_sends (kind = alert:<rule>, period = date|app|id). Thresholds come from AlertRules.
//   - purchase: a failed purchase (purchase.failed, except user cancelled / pending parental approval), on the
//     first occurrence of the day
//   - error: a payment-related error (AlertRules.PaymentErrors, e.g. purchase / restore / products.load) on the
//     first occurrence; any other error when it is seen for the first time ever, or affects >= SpikeUsers users
//     within an hour
//   - asc_stale: the App Store Connect daily report has not been updated for more than ReportStaleDays days
//     (expired key, API change, ...)
//   - silent: an app has sent no events for SilentHours hours although the previous 7 days had data (SDK or
//     upload pipeline problem; a low-traffic app can go quiet for up to ~38 hours overnight)

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/i18n"
	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

type Alerter struct {
	DB     *sql.DB
	Ledger Ledger
	Sender Sender
	Config Config
	Now    func() time.Time
}

type alert struct {
	rule, key string // dedup: the same rule + key is sent once a day
	view      *View
}

func (a *Alerter) Loop() {
	for {
		a.Tick()
		time.Sleep(5 * time.Minute)
	}
}

// Tick checks once and sends alerts not sent yet; failed sends are retried on the next check.
func (a *Alerter) Tick() {
	now := a.Now().In(tz.Location())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	alerts, err := checkAlerts(ctx, a.DB, a.Config, now)
	if err != nil {
		slog.Error("check alerts", "error", err)
	}
	day := now.Format("2006-01-02")
	for _, al := range alerts {
		kind, period := "alert:"+al.rule, day+"|"+al.key
		sent, err := a.Ledger.ReportSent(kind, period)
		if err != nil {
			slog.Error("alert ledger", "kind", kind, "period", period, "error", err)
			continue
		}
		if sent {
			continue
		}
		html, err := al.view.HTML()
		if err != nil {
			slog.Error("render alert html, fallback to text", "error", err)
			html = ""
		}
		if err := a.Sender.Send(al.view.Subject, al.view.Text(), html); err != nil {
			slog.Error("send alert", "kind", kind, "period", period, "error", err)
			continue
		}
		if err := a.Ledger.MarkReportSent(kind, period, now); err != nil {
			slog.Error("mark alert sent", "kind", kind, "period", period, "error", err)
		}
		slog.Info("alert sent", "kind", kind, "period", period)
	}
}

// checkAlerts returns all current alerts (sent or not); a rule that fails is skipped and the rest are still returned.
func checkAlerts(ctx context.Context, db *sql.DB, cfg Config, now time.Time) ([]alert, error) {
	dayStart := tz.StartOfDay(now).UnixMilli()
	var out []alert
	var errs []string
	for _, app := range cfg.Apps {
		for _, check := range []func(context.Context, *sql.DB, AlertRules, App, int64, time.Time) ([]alert, error){purchaseAlerts, errorAlerts, silentAlerts} {
			as, err := check(ctx, db, cfg.Alerts, app, dayStart, now)
			if err != nil {
				errs = append(errs, app.Key+": "+err.Error())
				continue
			}
			out = append(out, as...)
		}
	}
	if al, err := ascStaleAlert(ctx, db, cfg.Alerts, now); err != nil {
		errs = append(errs, "asc: "+err.Error())
	} else if al != nil {
		out = append(out, *al)
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return out, nil
}

// Details of the latest occurrences: time, device model, OS, app version, storefront, install id.
const occurrenceCols = `substr(local_datetime(e.ts), 6, 11), COALESCE(m.name, e.device, ''),
	COALESCE(e.os_version, ''), COALESCE(e.app_version, ''), COALESCE(e.storefront, ''), e.install_id`

type occurrence struct{ time, device, os, version, storefront, install, detail, context string }

func (o occurrence) cells() []Cell {
	prefix := func(p, v string) string {
		if v == "" {
			return ""
		}
		return p + v
	}
	where := joinNonEmpty(" · ", o.device, prefix("iOS ", o.os), prefix("v", o.version), o.storefront)
	return []Cell{{Value: o.time}, {Value: joinNonEmpty(" · ", o.detail, o.context)}, {Value: where}, {Value: shortInstall(o.install)}}
}

func occurrenceColumns() []string {
	return []string{i18n.T("Time", "时间"), i18n.T("Details", "详情"), i18n.T("Device", "设备"), i18n.T("User", "用户")}
}

func shortInstall(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func purchaseAlerts(ctx context.Context, db *sql.DB, _ AlertRules, app App, dayStart int64, now time.Time) ([]alert, error) {
	rs, err := db.QueryContext(ctx, `
		SELECT COALESCE(json_extract(params, '$.reason'), ''), COUNT(*), COUNT(DISTINCT install_id)
		FROM events WHERE app = ? AND env = 'production' AND name = 'purchase.failed' AND received_at >= ?
			AND COALESCE(json_extract(params, '$.reason'), '') NOT IN ('user_cancelled', 'pending')
		GROUP BY 1 ORDER BY 2 DESC`, app.Key, dayStart)
	if err != nil {
		return nil, err
	}
	type group struct {
		reason   string
		n, users int
	}
	var groups []group
	for rs.Next() {
		var g group
		if err := rs.Scan(&g.reason, &g.n, &g.users); err != nil {
			rs.Close()
			return nil, err
		}
		groups = append(groups, g)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	var out []alert
	for _, g := range groups {
		occ, err := occurrences(ctx, db, `e.app = ? AND e.env = 'production' AND e.name = 'purchase.failed' AND e.received_at >= ?
			AND COALESCE(json_extract(e.params, '$.reason'), '') = ?`, "$.product", app.Key, dayStart, g.reason)
		if err != nil {
			return nil, err
		}
		reason := orDash(g.reason)
		out = append(out, alert{rule: "purchase", key: app.Key + "|" + g.reason, view: alertView(
			i18n.T(fmt.Sprintf("Purchase failed · %s · %s · %s, %s", app.Name, reason, people(g.users), units(g.n, "time", "times", "")),
				fmt.Sprintf("购买失败 · %s · %s · %d 人 %d 次", app.Name, reason, g.users, g.n)),
			i18n.T(app.Name+" purchase failed: "+reason, app.Name+" 购买失败："+reason), now,
			Stat{Label: i18n.T("Users affected today", "今天受影响用户"), Cell: Cell{Value: people(g.users)}},
			i18n.T(fmt.Sprintf("%s in total; reason %s%s", units(g.n, "failure", "failures", ""), reason, purchaseHint(g.reason)),
				fmt.Sprintf("共失败 %d 次；原因 %s%s", g.n, reason, purchaseHint(g.reason))),
			occ)})
	}
	return out, nil
}

func purchaseHint(reason string) string {
	switch reason {
	case "product_missing":
		return paren(i18n.T("products did not load from the App Store, so users cannot buy", "商品没从 App Store 加载到，用户点了也买不了"))
	case "verification":
		return paren(i18n.T("transaction verification failed", "交易校验失败"))
	case "system_error":
		return paren(i18n.T("StoreKit or network error", "StoreKit 或网络错误"))
	}
	return ""
}

func errorAlerts(ctx context.Context, db *sql.DB, rules AlertRules, app App, dayStart int64, now time.Time) ([]alert, error) {
	// every error id received today: count / users today, users in the last hour, whether it was seen before
	rs, err := db.QueryContext(ctx, `
		SELECT COALESCE(json_extract(params, '$.id'), '') AS id, COUNT(*), COUNT(DISTINCT install_id),
			COUNT(DISTINCT CASE WHEN received_at >= ? THEN install_id END),
			EXISTS (SELECT 1 FROM events o WHERE o.app = e.app AND o.env = 'production' AND o.name = 'error'
				AND o.received_at < ? AND json_extract(o.params, '$.id') = json_extract(e.params, '$.id'))
		FROM events e WHERE app = ? AND env = 'production' AND name = 'error' AND received_at >= ?
		GROUP BY 1 ORDER BY 2 DESC`, now.Add(-time.Hour).UnixMilli(), dayStart, app.Key, dayStart)
	if err != nil {
		return nil, err
	}
	type group struct {
		id                  string
		n, users, hourUsers int
		seen                bool
	}
	var groups []group
	for rs.Next() {
		var g group
		if err := rs.Scan(&g.id, &g.n, &g.users, &g.hourUsers, &g.seen); err != nil {
			rs.Close()
			return nil, err
		}
		groups = append(groups, g)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	var out []alert
	for _, g := range groups {
		var why string
		switch {
		case rules.PaymentErrors[g.id]:
			why = i18n.T("Payment-related error", "付费相关错误")
		case !g.seen:
			why = i18n.T("First occurrence of this error", "首次出现的错误")
		case g.hourUsers >= rules.SpikeUsers:
			why = i18n.T(people(g.hourUsers)+" affected in the last hour", fmt.Sprintf("近 1 小时影响 %d 人", g.hourUsers))
		default:
			continue
		}
		occ, err := occurrences(ctx, db, `e.app = ? AND e.env = 'production' AND e.name = 'error' AND e.received_at >= ?
			AND COALESCE(json_extract(e.params, '$.id'), '') = ?`, "$.message", app.Key, dayStart, g.id)
		if err != nil {
			return nil, err
		}
		id := orDash(g.id)
		out = append(out, alert{rule: "error", key: app.Key + "|" + g.id, view: alertView(
			i18n.T(fmt.Sprintf("Error · %s · %s · %s, %s", app.Name, id, people(g.users), units(g.n, "time", "times", "")),
				fmt.Sprintf("错误告警 · %s · %s · %d 人 %d 次", app.Name, id, g.users, g.n)),
			i18n.T(app.Name+" error: "+id, app.Name+" 错误："+id), now,
			Stat{Label: i18n.T("Users affected today", "今天受影响用户"), Cell: Cell{Value: people(g.users)}},
			i18n.T(fmt.Sprintf("%s; %s today", why, units(g.n, "time", "times", "")), fmt.Sprintf("%s；今天共 %d 次", why, g.n)),
			occ)})
	}
	return out, nil
}

// occurrences returns the latest 10 occurrences; detailPath is the params field shown as the details.
func occurrences(ctx context.Context, db *sql.DB, where, detailPath string, args ...any) ([]occurrence, error) {
	rs, err := db.QueryContext(ctx, `
		SELECT `+occurrenceCols+`, COALESCE(json_extract(e.params, '`+detailPath+`'), ''), COALESCE(json_extract(e.params, '$.context'), '')
		FROM events e LEFT JOIN device_models m ON m.identifier = e.device
		WHERE `+where+` ORDER BY e.ts DESC LIMIT 10`, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []occurrence
	for rs.Next() {
		var o occurrence
		if err := rs.Scan(&o.time, &o.device, &o.os, &o.version, &o.storefront, &o.install, &o.detail, &o.context); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rs.Err()
}

func silentAlerts(ctx context.Context, db *sql.DB, rules AlertRules, app App, _ int64, now time.Time) ([]alert, error) {
	var last sql.NullInt64
	var weekBefore int
	cut := now.Add(-time.Duration(rules.SilentHours) * time.Hour).UnixMilli()
	if err := db.QueryRowContext(ctx, `
		SELECT MAX(received_at), COUNT(CASE WHEN received_at BETWEEN ? AND ? THEN 1 END)
		FROM events WHERE app = ? AND env = 'production' AND ts >= ?`,
		now.Add(-time.Duration(rules.SilentHours+7*24)*time.Hour).UnixMilli(), cut, app.Key, now.Add(-30*24*time.Hour).UnixMilli()).Scan(&last, &weekBefore); err != nil {
		return nil, err
	}
	if weekBefore == 0 || (last.Valid && last.Int64 >= cut) {
		return nil, nil
	}
	since := time.UnixMilli(last.Int64).In(tz.Location())
	hours := int(now.Sub(since).Hours())
	return []alert{{rule: "silent", key: app.Key, view: alertView(
		i18n.T(fmt.Sprintf("Events stopped · %s · no data for %d hours", app.Name, hours),
			fmt.Sprintf("埋点中断 · %s · %d 小时没有收到数据", app.Name, hours)),
		i18n.T(app.Name+" stopped sending events", app.Name+" 埋点中断"), now,
		Stat{Label: i18n.T("Last received", "最后一次收到"), Cell: Cell{Value: since.Format(i18n.T("Jan 2 15:04", "01-02 15:04"))}},
		i18n.T(fmt.Sprintf("No production events for %d hours although the previous 7 days had data; check the SDK uploads, the ingest endpoint and the reverse proxy", hours),
			fmt.Sprintf("已 %d 小时没有收到正式环境埋点，此前 7 天有数据；检查 SDK 上报、ingest 接口和反向代理", hours)), nil)}}, nil
}

func ascStaleAlert(ctx context.Context, db *sql.DB, rules AlertRules, now time.Time) (*alert, error) {
	var latest sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT MAX(period) FROM asc_reports WHERE frequency = 'DAILY' AND status IN ('ok', 'empty')`).Scan(&latest); err != nil {
		return nil, err
	}
	if !latest.Valid { // not configured or never synced
		return nil, nil
	}
	today := now.Format("2006-01-02")
	if latest.String >= addDays(today, -rules.ReportStaleDays) {
		return nil, nil
	}
	return &alert{rule: "asc_stale", key: "asc", view: alertView(
		i18n.T("Report sync stuck · App Store Connect daily report stuck at "+latest.String,
			"报表同步卡住 · App Store Connect 日报停在 "+latest.String),
		i18n.T("App Store Connect daily report not updated", "App Store Connect 日报没有更新"), now,
		Stat{Label: i18n.T("Latest daily report", "最新日报"), Cell: Cell{Value: latest.String}},
		i18n.T(fmt.Sprintf("No new daily report for more than %d days (normally 1–2 days behind); check whether the App Store Connect API key has expired and look for errors in the sync log", rules.ReportStaleDays),
			fmt.Sprintf("超过 %d 天没有新的日报（正常滞后 1–2 天）；检查 ASC 密钥是否过期、同步日志里的报错", rules.ReportStaleDays)), nil)}, nil
}

func alertView(subject, title string, now time.Time, hero Stat, note string, occ []occurrence) *View {
	v := &View{Subject: subject, Kicker: i18n.T("Alert · ", "告警 · ") + now.Format(i18n.T("Jan 2 15:04", "01-02 15:04")), Title: title, Hero: hero, HeroNote: note,
		Footer: i18n.T("Checked every 5 minutes by HiwiKInsight · production data only · each problem is sent at most once a day",
			"由 HiwiKInsight 每 5 分钟检查一次 · 只统计正式环境（production）· 同一问题每天只发一次")}
	if len(occ) > 0 {
		t := Table{Title: i18n.T("Recent occurrences", "最近几次"), Columns: occurrenceColumns(),
			Note: i18n.T("User is the first 8 characters of the install ID; see the full timeline on the dashboard's user page",
				"用户为安装 ID 前 8 位，可在看板用户页查看完整轨迹")}
		for _, o := range occ {
			t.Rows = append(t.Rows, o.cells())
		}
		v.Tables = append(v.Tables, t)
	}
	return v
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

package metrics

// Dashboard metrics: overview / sessions_daily / users / event_trend / errors.
// The CLI and AI assistants can call them too; definitions are consistent with the other metrics.

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func init() {
	defs = append(defs,
		Def{"overview", "Overview KPIs for the current period and the preceding period of equal length: average DAU, active users in the range, truly new users, stickiness (average DAU / MAU as of `to`), sessions, daily sessions per user, median session duration, net revenue; plus a daily trend and user tiers", overview},
		Def{"sessions_daily", "Daily sessions, active users, sessions per user, and the median / P90 foreground duration (seconds) of sessions that ended that day", sessionsDaily},
		Def{"users", "Active users in the range; tiers are based on active days in the 28 days up to `to` (heavy >= 15, medium 5-14, light 2-4, once 1, dormant 0); parameters tier / paid=1 / new=1 / sort / offset / limit", users},
		Def{"event_trend", "Count and users of any event by day / week / month, optionally grouped by one parameter (at most 8 groups, the rest as (other)); parameters event / group_by / granularity=day|week|month", eventTrend},
		Def{"errors", "error events aggregated by id: count, affected users, versions involved, last occurrence and latest message; plus daily error counts", errorsMetric},
	)
}

func (p Params) arg(k string) string { return p.Args[k] }

// ---------- tiers ----------

// Tier classifies a user by active days in the last 28 days.
func Tier(days28 int) string {
	switch {
	case days28 >= 15:
		return "heavy"
	case days28 >= 5:
		return "medium"
	case days28 >= 2:
		return "light"
	case days28 == 1:
		return "once"
	}
	return "dormant"
}

var tierOrder = []string{"heavy", "medium", "light", "once", "dormant"}

// days28 returns each user's active days in the 28 days up to `to`.
func days28(ctx context.Context, db *sql.DB, p Params) (map[string]int, error) {
	users, err := userDays(ctx, db, p, addDays(p.To, -27), p.To)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for id, d := range users {
		out[id] = len(d)
	}
	return out, nil
}

// ---------- overview ----------

type periodStats struct {
	avgDAU, stickiness       float64
	activeUsers, newUsers    int
	sessions, activeUserDays int
	medianDuration           any
	net                      float64
}

func statsFor(ctx context.Context, db *sql.DB, p Params, from, to string) (*periodStats, error) {
	s := &periodStats{}
	users, err := userDays(ctx, db, p, addDays(to, -29), to)
	if err != nil {
		return nil, err
	}
	byDay := activeByDay(users)
	days := dayRange(from, to)
	sum := 0
	for _, d := range days {
		sum += len(byDay[d])
	}
	s.avgDAU = round1(float64(sum) / float64(max(len(days), 1)))
	s.activeUsers = unionRange(byDay, from, to)
	if mau := len(users); mau > 0 {
		s.stickiness = round1(s.avgDAU / float64(mau) * 100)
	}
	fresh, err := newUsers(ctx, db, p, from, to)
	if err != nil {
		return nil, err
	}
	s.newUsers = len(fresh)
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(sessions), 0), COUNT(*) FROM daily_active
		WHERE app = ? AND env = ? AND day BETWEEN ? AND ?`, p.App, p.Env, from, to).Scan(&s.sessions, &s.activeUserDays); err != nil {
		return nil, err
	}
	durs, err := sessionDurations(ctx, db, p, from, to)
	if err != nil {
		return nil, err
	}
	var all []int
	for _, v := range durs {
		all = append(all, v...)
	}
	s.medianDuration = quantile(all, 0.5)
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(revenue_sign * amount), 0) FROM appstore_events
		WHERE bundle_id = ? AND environment = ? AND revenue_sign != 0
			AND local_date(signed_at) BETWEEN ? AND ?`,
		p.Bundle, appleEnv[p.Env], from, to).Scan(&s.net); err != nil {
		return nil, err
	}
	s.net = math.Round(s.net*100) / 100
	return s, nil
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// sessionDurations returns the foreground seconds of sessions that ended on each day.
func sessionDurations(ctx context.Context, db *sql.DB, p Params, from, to string) (map[string][]int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT day, CAST(json_extract(params, '$.duration_s') AS INTEGER) FROM events
		WHERE app = ? AND env = ? AND name = 'session.ended' AND day BETWEEN ? AND ?`, p.App, p.Env, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]int{}
	for rows.Next() {
		var day string
		var d sql.NullInt64
		if err := rows.Scan(&day, &d); err != nil {
			return nil, err
		}
		if d.Valid {
			out[day] = append(out[day], int(d.Int64))
		}
	}
	return out, rows.Err()
}

// quantile returns the q-quantile, or nil when there is no data.
func quantile(v []int, q float64) any {
	if len(v) == 0 {
		return nil
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[int(float64(len(s)-1)*q+0.5)]
}

func overview(ctx context.Context, db *sql.DB, p Params) ([]Table, error) {
	n := dayDiff(p.From, p.To) + 1
	prevTo := addDays(p.From, -1)
	prevFrom := addDays(prevTo, -(n - 1))
	cur, err := statsFor(ctx, db, p, p.From, p.To)
	if err != nil {
		return nil, err
	}
	prev, err := statsFor(ctx, db, p, prevFrom, prevTo)
	if err != nil {
		return nil, err
	}
	perDay := func(s *periodStats) float64 {
		return round1(float64(s.sessions) / float64(max(s.activeUserDays, 1)))
	}
	kpis := Table{Name: "kpis", Columns: []string{"key", "current", "previous"}, Rows: [][]any{
		{"avg_dau", cur.avgDAU, prev.avgDAU},
		{"active_users", cur.activeUsers, prev.activeUsers},
		{"new_users", cur.newUsers, prev.newUsers},
		{"stickiness_pct", cur.stickiness, prev.stickiness},
		{"sessions", cur.sessions, prev.sessions},
		{"sessions_per_user_day", perDay(cur), perDay(prev)},
		{"median_session_s", cur.medianDuration, prev.medianDuration},
		{"net", cur.net, prev.net},
	}}
	period := Table{Name: "period", Columns: []string{"from", "to", "prev_from", "prev_to"},
		Rows: [][]any{{p.From, p.To, prevFrom, prevTo}}}

	// Daily trend (for sparklines)
	users, err := userDays(ctx, db, p, addDays(p.From, -29), p.To)
	if err != nil {
		return nil, err
	}
	byDay := activeByDay(users)
	fresh, err := newUsers(ctx, db, p, p.From, p.To)
	if err != nil {
		return nil, err
	}
	newByDay := map[string]int{}
	for _, d := range fresh {
		newByDay[d]++
	}
	sessByDay := map[string]int{}
	rows, err := db.QueryContext(ctx, `SELECT day, SUM(sessions) FROM daily_active WHERE app = ? AND env = ? AND day BETWEEN ? AND ? GROUP BY day`,
		p.App, p.Env, p.From, p.To)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d string
		var n int
		rows.Scan(&d, &n)
		sessByDay[d] = n
	}
	rows.Close()
	revByDay := map[string]float64{}
	rows, err = db.QueryContext(ctx, `
		SELECT local_date(signed_at) AS d, COALESCE(SUM(revenue_sign * amount), 0) FROM appstore_events
		WHERE bundle_id = ? AND environment = ? AND revenue_sign != 0 AND d BETWEEN ? AND ? GROUP BY d`,
		p.Bundle, appleEnv[p.Env], p.From, p.To)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d string
		var v float64
		rows.Scan(&d, &v)
		revByDay[d] = v
	}
	rows.Close()
	durs, err := sessionDurations(ctx, db, p, p.From, p.To)
	if err != nil {
		return nil, err
	}
	daily := Table{Name: "daily", Columns: []string{"day", "dau", "wau", "mau", "new_users", "sessions", "median_session_s", "net"}}
	union := func(end string, n int) int {
		set := map[string]bool{}
		for i := 0; i < n; i++ {
			for id := range byDay[addDays(end, -i)] {
				set[id] = true
			}
		}
		return len(set)
	}
	for _, d := range dayRange(p.From, p.To) {
		daily.Rows = append(daily.Rows, []any{d, len(byDay[d]), union(d, 7), union(d, 30), newByDay[d], sessByDay[d],
			quantile(durs[d], 0.5), math.Round(revByDay[d]*100) / 100})
	}

	// User tiers: active users in the range by active days in the last 28 days
	d28, err := days28(ctx, db, p)
	if err != nil {
		return nil, err
	}
	tierCount := map[string]int{}
	for id, days := range users {
		if days[len(days)-1] >= p.From {
			tierCount[Tier(d28[id])]++
		}
	}
	tiers := Table{Name: "tiers", Columns: []string{"tier", "users"}}
	for _, t := range tierOrder {
		tiers.Rows = append(tiers.Rows, []any{t, tierCount[t]})
	}
	return []Table{kpis, period, daily, tiers}, nil
}

// ---------- sessions_daily ----------

func sessionsDaily(ctx context.Context, db *sql.DB, p Params) ([]Table, error) {
	type agg struct{ sessions, users int }
	byDay := map[string]*agg{}
	rows, err := db.QueryContext(ctx, `
		SELECT day, COALESCE(SUM(sessions), 0), COUNT(*) FROM daily_active
		WHERE app = ? AND env = ? AND day BETWEEN ? AND ? GROUP BY day`, p.App, p.Env, p.From, p.To)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		a := &agg{}
		var d string
		if err := rows.Scan(&d, &a.sessions, &a.users); err != nil {
			rows.Close()
			return nil, err
		}
		byDay[d] = a
	}
	rows.Close()
	durs, err := sessionDurations(ctx, db, p, p.From, p.To)
	if err != nil {
		return nil, err
	}
	t := Table{Name: "daily", Columns: []string{"day", "sessions", "users", "sessions_per_user", "median_session_s", "p90_session_s"}}
	for _, d := range dayRange(p.From, p.To) {
		a := byDay[d]
		if a == nil {
			a = &agg{}
		}
		var per any
		if a.users > 0 {
			per = round1(float64(a.sessions) / float64(a.users))
		}
		t.Rows = append(t.Rows, []any{d, a.sessions, a.users, per, quantile(durs[d], 0.5), quantile(durs[d], 0.9)})
	}
	return []Table{t}, nil
}

// ---------- users ----------

var userSorts = map[string]string{
	"last_seen":   "last_seen_day DESC, active_days DESC",
	"first_seen":  "first_seen_day DESC",
	"active_days": "active_days DESC, last_seen_day DESC",
	"sessions":    "sessions DESC",
	"duration":    "duration_s DESC",
}

func users(ctx context.Context, db *sql.DB, p Params) ([]Table, error) {
	d28, err := days28(ctx, db, p)
	if err != nil {
		return nil, err
	}
	order, ok := userSorts[p.arg("sort")]
	if !ok {
		order = userSorts["last_seen"]
	}
	all, err := queryTable(ctx, db, "users", `
		SELECT s.install_id, s.first_seen_day, s.acquired_day, s.last_seen_day, s.active_days, s.sessions, s.duration_s,
			s.device_name, s.os_version, s.last_version, s.region, s.storefront, s.is_new, s.is_paid
		FROM v_user_summary s
		WHERE s.app = ? AND s.env = ?
			AND EXISTS (SELECT 1 FROM daily_active d WHERE d.app = s.app AND d.env = s.env AND d.install_id = s.install_id AND d.day BETWEEN ? AND ?)
		ORDER BY `+order, p.App, p.Env, p.From, p.To)
	if err != nil {
		return nil, err
	}
	tierCount := map[string]int{}
	paidCount, newCount := 0, 0
	out := Table{Name: "users", Columns: append([]string{"install_id", "tier", "days_28"}, all.Columns[1:]...)}
	for _, r := range all.Rows {
		id := fmt.Sprint(r[0])
		tier := Tier(d28[id])
		tierCount[tier]++
		paid, fresh := toInt(r[13]) == 1, toInt(r[12]) == 1
		if paid {
			paidCount++
		}
		if fresh {
			newCount++
		}
		if t := p.arg("tier"); t != "" && t != "all" && t != tier {
			continue
		}
		if p.arg("paid") == "1" && !paid {
			continue
		}
		if p.arg("new") == "1" && !fresh {
			continue
		}
		out.Rows = append(out.Rows, append([]any{id, tier, d28[id]}, r[1:]...))
	}
	total := len(out.Rows)
	offset, _ := strconv.Atoi(p.arg("offset"))
	limit := p.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset = min(max(offset, 0), total)
	out.Rows = out.Rows[offset:min(offset+limit, total)]
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	summary := Table{Name: "summary", Columns: []string{"total", "offset", "limit", "all_users", "paid_users", "new_users"},
		Rows: [][]any{{total, offset, limit, len(all.Rows), paidCount, newCount}}}
	tiers := Table{Name: "tiers", Columns: []string{"tier", "users"}}
	for _, t := range tierOrder {
		tiers.Rows = append(tiers.Rows, []any{t, tierCount[t]})
	}
	return []Table{summary, tiers, out}, nil
}

// ---------- event_trend ----------

var (
	eventNameRE = regexp.MustCompile(`^[a-z0-9_.]{1,64}$`)
	paramKeyRE  = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)
)

func bucketExpr(granularity string) string {
	switch granularity {
	case "week":
		return "date(day, '-' || ((CAST(strftime('%w', day) AS INTEGER) + 6) % 7) || ' days')" // Monday
	case "month":
		return "substr(day, 1, 7)"
	}
	return "day"
}

func eventTrend(ctx context.Context, db *sql.DB, p Params) ([]Table, error) {
	events, err := queryTable(ctx, db, "events", `
		SELECT name, COUNT(*) AS count, COUNT(DISTINCT install_id) AS users FROM events
		WHERE app = ? AND env = ? AND day BETWEEN ? AND ? GROUP BY name ORDER BY count DESC`, p.App, p.Env, p.From, p.To)
	if err != nil {
		return nil, err
	}
	event := p.arg("event")
	if event == "" && len(events.Rows) > 0 { // default to the most frequent business event, skipping SDK auto-collected ones
		event = fmt.Sprint(events.Rows[0][0])
		for _, r := range events.Rows {
			n := fmt.Sprint(r[0])
			if !strings.HasPrefix(n, "screen.") && !strings.HasPrefix(n, "session.") && !strings.HasPrefix(n, "app.") && n != "user.snapshot" {
				event = n
				break
			}
		}
	}
	if event != "" && !eventNameRE.MatchString(event) {
		return nil, fmt.Errorf("invalid event %q", event)
	}
	groupBy := p.arg("group_by")
	if groupBy != "" && !paramKeyRE.MatchString(groupBy) {
		return nil, fmt.Errorf("invalid group_by %q", groupBy)
	}
	granularity := p.arg("granularity")
	if granularity != "week" && granularity != "month" {
		granularity = "day"
	}
	params, err := queryTable(ctx, db, "params", `
		SELECT j.key AS key, COUNT(*) AS count, COUNT(DISTINCT j.value) AS distinct_values
		FROM events e, json_each(e.params) j
		WHERE e.app = ? AND e.env = ? AND e.day BETWEEN ? AND ? AND e.name = ?
		GROUP BY j.key ORDER BY count DESC`, p.App, p.Env, p.From, p.To, event)
	if err != nil {
		return nil, err
	}
	value := "'(all)'"
	if groupBy != "" {
		value = fmt.Sprintf("COALESCE(CAST(json_extract(params, '$.%s') AS TEXT), '(none)')", groupBy)
	}
	base := fmt.Sprintf(`
		WITH base AS (
			SELECT %s AS bucket, %s AS v, install_id FROM events
			WHERE app = ? AND env = ? AND day BETWEEN ? AND ? AND name = ?
		), top AS (SELECT v FROM base GROUP BY v ORDER BY COUNT(*) DESC LIMIT 8),
		g AS (SELECT bucket, CASE WHEN v IN (SELECT v FROM top) THEN v ELSE '(other)' END AS value, install_id FROM base)`,
		bucketExpr(granularity), value)
	args := []any{p.App, p.Env, p.From, p.To, event}
	series, err := queryTable(ctx, db, "series", base+`
		SELECT bucket, value, COUNT(*) AS count, COUNT(DISTINCT install_id) AS users FROM g GROUP BY 1, 2 ORDER BY 1, 3 DESC`, args...)
	if err != nil {
		return nil, err
	}
	breakdown, err := queryTable(ctx, db, "breakdown", base+`
		SELECT value, COUNT(*) AS count, COUNT(DISTINCT install_id) AS users FROM g GROUP BY 1 ORDER BY 2 DESC`, args...)
	if err != nil {
		return nil, err
	}
	// All buckets (including empty ones); the frontend uses them directly as the x axis
	buckets := Table{Name: "buckets", Columns: []string{"bucket"}}
	seen := map[string]bool{}
	for _, d := range dayRange(p.From, p.To) {
		var b string
		switch granularity {
		case "week":
			t := parseDay(d)
			b = t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7)).Format("2006-01-02")
		case "month":
			b = d[:7]
		default:
			b = d
		}
		if !seen[b] {
			seen[b] = true
			buckets.Rows = append(buckets.Rows, []any{b})
		}
	}
	selected := Table{Name: "selected", Columns: []string{"event", "group_by", "granularity"},
		Rows: [][]any{{event, groupBy, granularity}}}
	return []Table{selected, buckets, series, breakdown, params, events}, nil
}

// ---------- errors ----------

func errorsMetric(ctx context.Context, db *sql.DB, p Params) ([]Table, error) {
	args := []any{p.App, p.Env, p.From, p.To}
	const where = `app = ? AND env = ? AND name = 'error' AND day BETWEEN ? AND ?`
	summary, err := queryTable(ctx, db, "summary", `
		SELECT COUNT(*) AS count, COUNT(DISTINCT install_id) AS users,
			COUNT(DISTINCT COALESCE(json_extract(params, '$.id'), '(none)')) AS kinds
		FROM events WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	daily, err := queryTable(ctx, db, "daily", `SELECT day, COUNT(*) AS count, COUNT(DISTINCT install_id) AS users FROM events WHERE `+where+` GROUP BY day`, args...)
	if err != nil {
		return nil, err
	}
	// Fill missing days with zero
	have := map[string][]any{}
	for _, r := range daily.Rows {
		have[fmt.Sprint(r[0])] = r
	}
	daily.Rows = nil
	for _, d := range dayRange(p.From, p.To) {
		if r, ok := have[d]; ok {
			daily.Rows = append(daily.Rows, r)
		} else {
			daily.Rows = append(daily.Rows, []any{d, 0, 0})
		}
	}
	list, err := queryTable(ctx, db, "errors", `
		WITH e AS (
			SELECT COALESCE(json_extract(params, '$.id'), '(none)') AS id, json_extract(params, '$.category') AS category,
				json_extract(params, '$.message') AS message, install_id, app_version, ts
			FROM events WHERE `+where+`
		)
		SELECT id, MAX(category) AS category, COUNT(*) AS count, COUNT(DISTINCT install_id) AS users,
			GROUP_CONCAT(DISTINCT app_version) AS versions,
			local_datetime(MAX(ts)) AS last_time_local,
			(SELECT message FROM e e2 WHERE e2.id = e.id ORDER BY ts DESC LIMIT 1) AS last_message
		FROM e GROUP BY id ORDER BY users DESC, count DESC`, args...)
	if err != nil {
		return nil, err
	}
	for i, r := range list.Rows {
		if v, ok := r[4].(string); ok {
			vs := strings.Split(v, ",")
			sort.Sort(sort.Reverse(sort.StringSlice(vs)))
			list.Rows[i][4] = strings.Join(vs, ", ")
		}
	}
	return []Table{summary, daily, list}, nil
}

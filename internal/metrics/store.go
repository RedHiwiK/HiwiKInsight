package metrics

// Store conversion (App Store Connect analytics reports): impressions → product page views → first-time downloads,
// split by source and country. These reports lag behind sales reports (about 2-3 days), so the window ends on the
// latest day with data and its length is the number of days in the selected range.

import (
	"context"
	"database/sql"
)

func init() {
	defs = append(defs,
		Def{"store_funnel", "Store conversion (App Store Connect analytics reports): impressions (unique devices) -> product page views (unique devices) -> first-time downloads; conversion rate = first-time downloads / unique impressions (same definition as App Store Connect), product page conversion = first-time downloads / unique product page views; the window ends on the latest report day, its length is the number of days in from~to, and it is compared with the preceding window of equal length; split by app, source (search / browse / app referrer / web referrer / other) and country. scope=all covers all apps, otherwise only the current app", storeFunnel},
	)
}

// Both tables share the same grain: the impressions and downloads tables are each replaced per (app, date),
// so they are combined with UNION ALL here and then aggregated.
const funnelSrc = `
	WITH f AS (
		SELECT date, bundle_id, source, territory, impressions, impressions_unique, page_views, page_views_unique,
			0 AS first_downloads, 0 AS redownloads
		FROM asc_store_engagement
		UNION ALL
		SELECT date, bundle_id, source, territory, 0, 0, 0, 0, first_downloads, redownloads
		FROM asc_store_downloads
	)`

func storeFunnel(ctx context.Context, db *sql.DB, p Params) ([]Table, error) {
	all := p.arg("scope") == "all"
	where, args := "1 = 1", []any{}
	if !all {
		where, args = "bundle_id = ?", []any{p.Bundle}
	}
	var latest, earliest sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT MAX(date), MIN(date) FROM asc_store_engagement WHERE `+where, args...).Scan(&latest, &earliest); err != nil {
		return nil, err
	}
	status := Table{Name: "status", Columns: []string{"latest_date", "earliest_date", "synced"},
		Rows: [][]any{{nullStr(latest), nullStr(earliest), latest.Valid}}}
	if !latest.Valid {
		return []Table{status}, nil
	}

	n := 28
	if p.From != "" && p.To != "" && p.From <= p.To {
		n = dayDiff(p.From, p.To) + 1
	}
	to := latest.String
	if p.To != "" && p.To < to {
		to = p.To
	}
	from := addDays(to, -(n - 1))
	prevTo := addDays(from, -1)
	prevFrom := addDays(prevTo, -(n - 1))
	// Reports only have data from the day they were enabled (until historical snapshots are backfilled);
	// a period-over-period comparison is meaningless when the previous window is incomplete.
	window := Table{Name: "window", Columns: []string{"from", "to", "prev_from", "prev_to", "days", "prev_complete"},
		Rows: [][]any{{from, to, prevFrom, prevTo, n, earliest.String <= prevFrom}}}

	// ---- per-app totals (including the previous window) ----
	w, a := where+" AND date BETWEEN ? AND ?", args
	type sums struct{ imp, impU, pv, pvU, first, re int }
	byApp := func(from, to string) (map[string]*sums, error) {
		rows, err := db.QueryContext(ctx, funnelSrc+`
			SELECT bundle_id, SUM(impressions), SUM(impressions_unique), SUM(page_views), SUM(page_views_unique),
				SUM(first_downloads), SUM(redownloads)
			FROM f WHERE `+w+` GROUP BY bundle_id`, append(a, from, to)...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := map[string]*sums{}
		for rows.Next() {
			var b string
			s := &sums{}
			if err := rows.Scan(&b, &s.imp, &s.impU, &s.pv, &s.pvU, &s.first, &s.re); err != nil {
				return nil, err
			}
			out[b] = s
		}
		return out, rows.Err()
	}
	cur, err := byApp(from, to)
	if err != nil {
		return nil, err
	}
	prev, err := byApp(prevFrom, prevTo)
	if err != nil {
		return nil, err
	}
	apps := Table{Name: "apps", Columns: []string{"bundle_id", "impressions", "impressions_unique", "page_views", "page_views_unique",
		"first_downloads", "redownloads", "view_rate", "cvr", "page_cvr", "prev_impressions_unique", "prev_first_downloads", "prev_cvr"}}
	for b, s := range cur {
		ps := prev[b]
		if ps == nil {
			ps = &sums{}
		}
		apps.Rows = append(apps.Rows, []any{b, s.imp, s.impU, s.pv, s.pvU, s.first, s.re,
			pct(s.pvU, s.impU), pct(s.first, s.impU), pct(s.first, s.pvU), ps.impU, ps.first, pct(ps.first, ps.impU)})
	}
	sortRows(apps.Rows, 2)

	// ---- by day ----
	daily, err := queryTable(ctx, db, "daily", funnelSrc+`
		SELECT date, bundle_id, SUM(impressions_unique) AS impressions_unique, SUM(page_views_unique) AS page_views_unique,
			SUM(first_downloads) AS first_downloads
		FROM f WHERE `+w+` GROUP BY date, bundle_id ORDER BY date, bundle_id`, append(a, from, to)...)
	if err != nil {
		return nil, err
	}

	// ---- by source ----
	sources, err := queryTable(ctx, db, "sources", funnelSrc+`
		SELECT source, SUM(impressions_unique) AS impressions_unique, SUM(page_views_unique) AS page_views_unique,
			SUM(first_downloads) AS first_downloads,
			ROUND(100.0 * SUM(first_downloads) / NULLIF(SUM(impressions_unique), 0), 1) AS cvr
		FROM f WHERE `+w+` GROUP BY source ORDER BY impressions_unique DESC`, append(a, from, to)...)
	if err != nil {
		return nil, err
	}

	// ---- by country (top 15) ----
	territories, err := queryTable(ctx, db, "territories", funnelSrc+`
		SELECT territory, SUM(impressions_unique) AS impressions_unique, SUM(page_views_unique) AS page_views_unique,
			SUM(first_downloads) AS first_downloads,
			ROUND(100.0 * SUM(first_downloads) / NULLIF(SUM(impressions_unique), 0), 1) AS cvr
		FROM f WHERE `+w+` GROUP BY territory ORDER BY impressions_unique DESC, first_downloads DESC LIMIT 15`, append(a, from, to)...)
	if err != nil {
		return nil, err
	}
	return []Table{status, window, apps, daily, sources, territories}, nil
}

// sortRows sorts by column col (an integer) in descending order; only used for small tables.
func sortRows(rows [][]any, col int) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && toInt(rows[j][col]) > toInt(rows[j-1][col]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

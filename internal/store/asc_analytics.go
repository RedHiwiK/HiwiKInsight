package store

import (
	"fmt"
	"time"
)

// FunnelRecord is one cell of the store funnel (date x app x source x country).
type FunnelRecord struct {
	Date, BundleID, Source, Territory string
	Impressions, ImpressionsUnique    int
	PageViews, PageViewsUnique        int
	FirstDownloads, Redownloads       int
}

// AnalyticsInstanceIDs returns the report instances that have already been processed.
func (s *Store) AnalyticsInstanceIDs() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT instance_id FROM asc_analytics_instances`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ApplyAnalyticsInstance stores one report instance: every (app, date) it covers is replaced as a whole,
// and the instance is marked as processed. discovery = true writes the engagement table, otherwise the downloads table.
func (s *Store) ApplyAnalyticsInstance(instanceID, report, bundleID, processingDate string, discovery bool, recs []FunnelRecord) error {
	table := "asc_store_downloads"
	if discovery {
		table = "asc_store_engagement"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cleared := map[[2]string]bool{}
	for _, r := range recs {
		k := [2]string{r.BundleID, r.Date}
		if cleared[k] {
			continue
		}
		cleared[k] = true
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE bundle_id = ? AND date = ?`, table), r.BundleID, r.Date); err != nil {
			return err
		}
	}
	for _, r := range recs {
		if discovery {
			if r.Impressions == 0 && r.PageViews == 0 {
				continue
			}
			_, err = tx.Exec(`INSERT INTO asc_store_engagement
				(date, bundle_id, source, territory, impressions, impressions_unique, page_views, page_views_unique)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				r.Date, r.BundleID, r.Source, r.Territory, r.Impressions, r.ImpressionsUnique, r.PageViews, r.PageViewsUnique)
		} else {
			if r.FirstDownloads == 0 && r.Redownloads == 0 {
				continue
			}
			_, err = tx.Exec(`INSERT INTO asc_store_downloads
				(date, bundle_id, source, territory, first_downloads, redownloads) VALUES (?, ?, ?, ?, ?, ?)`,
				r.Date, r.BundleID, r.Source, r.Territory, r.FirstDownloads, r.Redownloads)
		}
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO asc_analytics_instances (instance_id, report, bundle_id, processing_date, row_count, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (instance_id) DO UPDATE SET row_count = excluded.row_count, fetched_at = excluded.fetched_at`,
		instanceID, report, bundleID, processingDate, len(recs), time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

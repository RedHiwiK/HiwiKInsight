package store

import "time"

// ASCApp is an app in the App Store Connect account (used to map report rows to a bundle ID).
type ASCApp struct {
	AppleID, BundleID, SKU, Name string
}

// SalesRecord is one sales report row, already mapped to an app and converted to the base currency.
type SalesRecord struct {
	BundleID, SKU, ParentSKU, Title, ProductType, Category string
	Units, ProceedsPerUnit                                 float64
	ProceedsCurrency                                       string
	Proceeds                                               *float64
	CustomerPrice                                          float64
	CustomerCurrency, Country, AppleID                     string
	Subscription, SubPeriod                                string
}

func (s *Store) UpsertASCApps(apps []ASCApp) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	for _, a := range apps {
		if _, err := tx.Exec(`
			INSERT INTO asc_apps (apple_id, bundle_id, sku, name, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (apple_id) DO UPDATE SET bundle_id = excluded.bundle_id, sku = excluded.sku,
				name = excluded.name, updated_at = excluded.updated_at`,
			a.AppleID, a.BundleID, a.SKU, a.Name, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReplaceSales replaces all report rows of a period and records the fetch status (ok / empty).
func (s *Store) ReplaceSales(frequency, period, status string, rows []SalesRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM asc_sales WHERE frequency = ? AND period = ?`, frequency, period); err != nil {
		return err
	}
	month := period[:7]
	for _, r := range rows {
		if _, err := tx.Exec(`
			INSERT INTO asc_sales (frequency, period, month, bundle_id, sku, parent_sku, title, product_type, category,
				units, proceeds_per_unit, proceeds_currency, proceeds, customer_price, customer_currency,
				country, apple_id, subscription, sub_period)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			frequency, period, month, r.BundleID, r.SKU, r.ParentSKU, r.Title, r.ProductType, r.Category,
			r.Units, r.ProceedsPerUnit, r.ProceedsCurrency, r.Proceeds, r.CustomerPrice, r.CustomerCurrency,
			r.Country, r.AppleID, r.Subscription, r.SubPeriod); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO asc_reports (frequency, period, status, row_count, fetched_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (frequency, period) DO UPDATE SET status = excluded.status, row_count = excluded.row_count,
			fetched_at = excluded.fetched_at`,
		frequency, period, status, len(rows), time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// ASCReportStatus returns the processed periods of a frequency → status.
func (s *Store) ASCReportStatus(frequency string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT period, status FROM asc_reports WHERE frequency = ?`, frequency)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, st string
		if err := rows.Scan(&p, &st); err != nil {
			return nil, err
		}
		out[p] = st
	}
	return out, rows.Err()
}

package store

import "time"

// ReportSent reports whether the report for a period has already been sent.
func (s *Store) ReportSent(kind, period string) (bool, error) {
	var sent bool
	err := s.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM report_sends WHERE kind = ? AND period = ?)`, kind, period).Scan(&sent)
	return sent, err
}

// MarkReportSent records that the report for a period has been sent.
func (s *Store) MarkReportSent(kind, period string, at time.Time) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO report_sends (kind, period, sent_at) VALUES (?, ?, ?)`, kind, period, at.UnixMilli())
	return err
}

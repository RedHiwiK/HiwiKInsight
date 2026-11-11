package store

import "time"

// Delivery is one transaction message waiting for, or sent through, one channel.
// The message is rendered when the notification arrives, so a retry sends the same text.
type Delivery struct {
	NotificationUUID string
	Channel          string
	Subject          string
	Text             string
	HTML             string
	Attempts         int
}

// Enqueue stores pending deliveries. A delivery that already exists, from an earlier
// copy of the same notification, is left as it is.
func (s *Store) Enqueue(ds []Delivery) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	for _, d := range ds {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO notification_deliveries
			(notification_uuid, channel, subject, text_body, html_body, next_attempt_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			d.NotificationUUID, d.Channel, d.Subject, d.Text, d.HTML, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DueDeliveries returns a channel's undelivered messages that are due at now and have
// been tried fewer than maxAttempts times, oldest first.
func (s *Store) DueDeliveries(channel string, now time.Time, maxAttempts int) ([]Delivery, error) {
	rows, err := s.db.Query(`
		SELECT notification_uuid, channel, subject, text_body, html_body, attempts
		FROM notification_deliveries
		WHERE channel = ? AND delivered_at IS NULL AND next_attempt_at <= ? AND attempts < ?
		ORDER BY next_attempt_at`, channel, now.UnixMilli(), maxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ds []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.NotificationUUID, &d.Channel, &d.Subject, &d.Text, &d.HTML, &d.Attempts); err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	return ds, rows.Err()
}

// MarkDelivered records a successful send.
func (s *Store) MarkDelivered(d Delivery, at time.Time) error {
	_, err := s.db.Exec(`
		UPDATE notification_deliveries SET attempts = attempts + 1, delivered_at = ?, last_error = ''
		WHERE notification_uuid = ? AND channel = ?`, at.UnixMilli(), d.NotificationUUID, d.Channel)
	return err
}

// MarkFailed records a failed send and when to try again.
func (s *Store) MarkFailed(d Delivery, sendErr error, retryAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE notification_deliveries SET attempts = attempts + 1, next_attempt_at = ?, last_error = ?
		WHERE notification_uuid = ? AND channel = ?`, retryAt.UnixMilli(), sendErr.Error(), d.NotificationUUID, d.Channel)
	return err
}

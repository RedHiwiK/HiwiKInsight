package store

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

// Day returns the calendar day (YYYY-MM-DD) of a millisecond timestamp in the reporting time zone.
func Day(ms int64) string { return tz.Day(ms) }

// Batch is one normalized upload (ingest has already validated, truncated and clock-corrected it).
type Batch struct {
	App        string
	InstallID  string
	Env        string
	Context    map[string]string
	ReceivedAt int64
	Events     []AnalyticsEvent
}

type AnalyticsEvent struct {
	ID      string
	Name    string
	TS      int64 // clock-corrected
	Session string
	Params  map[string]string
}

// WriteBatches writes events in one transaction and maintains installs / daily_active / purchase_attempts.
// Duplicate event_ids are ignored and never counted twice in derived tables. Returns the number of newly written events.
func (s *Store) WriteBatches(batches []*Batch) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	insEvent, err := tx.Prepare(`
		INSERT OR IGNORE INTO events
		(event_id, app, env, install_id, session_id, name, ts, day, received_at,
		 app_version, build, os, os_version, device, locale, language, region, storefront, appearance, text_size, params)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insEvent.Close()
	upDaily, err := tx.Prepare(`
		INSERT INTO daily_active (app, env, day, install_id, sessions, duration_s, events)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT (app, env, day, install_id) DO UPDATE SET
			sessions = sessions + excluded.sessions,
			duration_s = duration_s + excluded.duration_s,
			events = events + 1`)
	if err != nil {
		return 0, err
	}
	defer upDaily.Close()

	written := 0
	for _, b := range batches {
		// An install that has sent sandbox / xcode data can only be a development device: when a TestFlight build
		// fails to get AppTransaction, the SDK (before 0.1.3) falls back to production, so record it as sandbox
		// here (for historical data see ReclassifyMixedInstalls)
		if b.Env == "production" {
			var dev bool
			if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM events
				WHERE app = ? AND install_id = ? AND env IN ('sandbox', 'xcode'))`, b.App, b.InstallID).Scan(&dev); err != nil {
				return 0, err
			}
			if dev {
				b.Env = "sandbox"
			}
		}
		c := b.Context
		minTS, maxTS := int64(0), int64(0)
		var originalDownload int64
		var priorUsage bool
		for _, e := range b.Events {
			params, _ := json.Marshal(e.Params)
			res, err := insEvent.Exec(e.ID, b.App, b.Env, b.InstallID, nullIfEmpty(e.Session), e.Name, e.TS, Day(e.TS), b.ReceivedAt,
				c["app_version"], c["build"], c["os"], c["os_version"], c["device"], c["locale"], c["language"],
				c["region"], c["storefront"], c["appearance"], c["text_size"], string(params))
			if err != nil {
				return 0, err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				continue // duplicate event caused by a retry
			}
			written++
			if minTS == 0 || e.TS < minTS {
				minTS = e.TS
			}
			if e.TS > maxTS {
				maxTS = e.TS
			}

			sessions, duration := 0, 0
			switch e.Name {
			case "session.started":
				sessions = 1
			case "session.ended":
				duration, _ = strconv.Atoi(e.Params["duration_s"])
				duration = max(0, min(duration, 86400))
			case "app.installed":
				originalDownload, _ = strconv.ParseInt(e.Params["original_download_ts"], 10, 64)
				priorUsage = e.Params["prior_usage"] == "1"
			case "app.acquired":
				// The original download time was not available at app.installed; the SDK (0.1.3+) sends it later
				originalDownload, _ = strconv.ParseInt(e.Params["original_download_ts"], 10, 64)
			case "purchase.started":
				if token := strings.ToLower(e.Params["token"]); token != "" {
					if _, err := tx.Exec(`
						INSERT OR IGNORE INTO purchase_attempts
						(token, app, env, install_id, context, product, ts, app_version, device, os_version)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
						token, b.App, b.Env, b.InstallID, e.Params["context"], e.Params["product"], e.TS,
						c["app_version"], c["device"], c["os_version"]); err != nil {
						return 0, err
					}
				}
			}
			if _, err := upDaily.Exec(b.App, b.Env, Day(e.TS), b.InstallID, sessions, duration); err != nil {
				return 0, err
			}
		}
		if minTS == 0 {
			continue
		}

		// installs: keep the earliest first seen, take the latest value of every other attribute
		if _, err := tx.Exec(`
			INSERT INTO installs (app, install_id, env, first_seen, acquired_at, first_version, last_seen, last_version,
				device, os_version, region, storefront, language)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (app, install_id) DO UPDATE SET
				env = excluded.env,
				first_seen = MIN(first_seen, excluded.first_seen),
				acquired_at = MIN(acquired_at, excluded.first_seen),
				last_seen = MAX(last_seen, excluded.last_seen),
				last_version = COALESCE(excluded.last_version, last_version),
				device = COALESCE(excluded.device, device),
				os_version = COALESCE(excluded.os_version, os_version),
				region = COALESCE(excluded.region, region),
				storefront = COALESCE(excluded.storefront, storefront),
				language = COALESCE(excluded.language, language)`,
			b.App, b.InstallID, b.Env, minTS, minTS, nullIfEmpty(c["app_version"]), maxTS, nullIfEmpty(c["app_version"]),
			nullIfEmpty(c["device"]), nullIfEmpty(c["os_version"]), nullIfEmpty(c["region"]),
			nullIfEmpty(c["storefront"]), nullIfEmpty(c["language"])); err != nil {
			return 0, err
		}
		// Existing users who installed the app before the SDK was added: correct the acquisition time with the App Store original download time
		if originalDownload > 0 {
			if _, err := tx.Exec(`UPDATE installs SET acquired_at = MIN(acquired_at, ?) WHERE app = ? AND install_id = ?`,
				originalDownload, b.App, b.InstallID); err != nil {
				return 0, err
			}
		} else if priorUsage {
			// No original download time, but the app reports prior usage on this device (SDK 0.1.2+): set the
			// acquisition time to the day before first seen, only to exclude these existing users from
			// "truly new users"; the real acquisition day is unknown
			if _, err := tx.Exec(`UPDATE installs SET acquired_at = MIN(acquired_at, first_seen - 86400000) WHERE app = ? AND install_id = ?`,
				b.App, b.InstallID); err != nil {
				return 0, err
			}
		}
	}
	return written, tx.Commit()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// PurgeEvents deletes raw events older than before; installs / daily_active / purchase_attempts are kept forever.
func (s *Store) PurgeEvents(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM events WHERE ts < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Writer merges Batches from all requests and writes them from a single goroutine, committing every second or every 500 events.
type Writer struct {
	store *Store
	ch    chan *Batch
	done  chan struct{}
	once  sync.Once
}

func NewWriter(s *Store) *Writer {
	w := &Writer{store: s, ch: make(chan *Batch, 1024), done: make(chan struct{})}
	go w.loop()
	return w
}

// Enqueue enqueues without blocking; returns false when the queue is full, and the caller replies 503 so the SDK backs off and retries.
func (w *Writer) Enqueue(b *Batch) bool {
	select {
	case w.ch <- b:
		return true
	default:
		return false
	}
}

// Close stops accepting batches and flushes the remaining data.
func (w *Writer) Close() {
	w.once.Do(func() { close(w.ch) })
	<-w.done
}

func (w *Writer) loop() {
	defer close(w.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var pending []*Batch
	count := 0
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if _, err := w.store.WriteBatches(pending); err != nil {
			slog.Error("write analytics batches failed", "error", err, "batches", len(pending), "events", count)
		}
		pending, count = nil, 0
	}
	for {
		select {
		case b, ok := <-w.ch:
			if !ok {
				flush()
				return
			}
			pending = append(pending, b)
			count += len(b.Events)
			if count >= 500 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Attribution is the in-app context an App Store transaction is linked to via appAccountToken.
type Attribution struct {
	App          string
	InstallID    string
	Context      string
	Product      string
	AttemptAt    int64
	AppVersion   string
	Device       string // human-readable device model name
	OSVersion    string
	InstallDay   int // day N since install (by date in the reporting time zone; the acquisition day is day 1)
	PaywallViews int // paywall views before starting the purchase (including the current one)
	AcquiredAt   int64
}

// FindAttribution looks up attribution by token; returns nil, nil when there is no record.
func (s *Store) FindAttribution(token string) (*Attribution, error) {
	token = strings.ToLower(token)
	if token == "" {
		return nil, nil
	}
	var a Attribution
	var appVersion, device, osVersion sql.NullString
	var acquired sql.NullInt64
	err := s.ro.QueryRow(`
		SELECT p.app, p.install_id, p.context, p.product, p.ts, p.app_version,
			COALESCE(m.name, p.device), p.os_version, i.acquired_at
		FROM purchase_attempts p
		LEFT JOIN device_models m ON m.identifier = p.device
		LEFT JOIN installs i ON i.app = p.app AND i.install_id = p.install_id
		WHERE p.token = ?`, token).
		Scan(&a.App, &a.InstallID, &a.Context, &a.Product, &a.AttemptAt, &appVersion, &device, &osVersion, &acquired)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.AppVersion, a.Device, a.OSVersion = appVersion.String, device.String, osVersion.String
	if acquired.Valid {
		a.AcquiredAt = acquired.Int64
		a.InstallDay = daysBetween(acquired.Int64, a.AttemptAt) + 1
	}
	err = s.ro.QueryRow(`
		SELECT COUNT(*) FROM events
		WHERE app = ? AND install_id = ? AND name = 'paywall.shown' AND ts <= ?`,
		a.App, a.InstallID, a.AttemptAt).Scan(&a.PaywallViews)
	return &a, err
}

func daysBetween(fromMS, toMS int64) int {
	from, _ := tz.ParseDay(Day(fromMS))
	to, _ := tz.ParseDay(Day(toMS))
	return int(to.Sub(from).Hours() / 24)
}

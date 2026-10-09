package report

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/tz"
)

type Sender interface {
	Send(subject, textBody, htmlBody string) error
}

// Ledger records which reports have been sent (implemented by store.Store).
type Ledger interface {
	ReportSent(kind, period string) (bool, error)
	MarkReportSent(kind, period string, at time.Time) error
}

// Scheduler sends the previous day's daily report after Hour o'clock (reporting time zone), plus last week's
// weekly report on Mondays. Every 5 minutes it checks for reports that are due and not yet sent, so restarts
// and deploys neither resend nor skip a day; a failed send is retried after 30 minutes.
type Scheduler struct {
	DB     *sql.DB
	Ledger Ledger
	Sender Sender
	Config Config
	Hour   int // hour of the day (reporting time zone) after which reports are sent
	Daily  bool
	Weekly bool
	Now    func() time.Time

	failedAt map[string]time.Time
}

func (s *Scheduler) Loop() {
	for {
		s.Tick()
		time.Sleep(5 * time.Minute)
	}
}

func (s *Scheduler) Tick() {
	now := s.Now().In(tz.Location())
	if now.Hour() < s.Hour {
		return
	}
	today := now.Format("2006-01-02")
	if s.Daily {
		s.sendOnce(now, Daily, addDays(today, -1))
	}
	if s.Weekly && now.Weekday() == time.Monday {
		s.sendOnce(now, Weekly, addDays(today, -7))
	}
}

func (s *Scheduler) sendOnce(now time.Time, kind Kind, period string) {
	key := string(kind) + "|" + period
	if t, ok := s.failedAt[key]; ok && now.Sub(t) < 30*time.Minute {
		return
	}
	sent, err := s.Ledger.ReportSent(string(kind), period)
	if err != nil {
		slog.Error("report ledger", "kind", kind, "period", period, "error", err)
		return
	}
	if sent {
		return
	}
	if err := s.send(now, kind, period); err != nil {
		if s.failedAt == nil {
			s.failedAt = map[string]time.Time{}
		}
		s.failedAt[key] = now
		slog.Error("send report", "kind", kind, "period", period, "error", err)
		return
	}
	if err := s.Ledger.MarkReportSent(string(kind), period, now); err != nil {
		slog.Error("mark report sent", "kind", kind, "period", period, "error", err)
	}
	slog.Info("report sent", "kind", kind, "period", period)
}

// SendNow sends the report of a period right away (manual resend / test send) without recording it, so the
// scheduled send is unaffected.
func (s *Scheduler) SendNow(kind Kind, period string) error {
	return s.send(s.Now().In(tz.Location()), kind, period)
}

func (s *Scheduler) send(now time.Time, kind Kind, period string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	v, err := Build(ctx, s.DB, s.Config, kind, period, now.Format("2006-01-02"))
	if err != nil {
		return err
	}
	html, err := v.HTML()
	if err != nil {
		slog.Error("render report html, fallback to text", "error", err)
		html = ""
	}
	return s.Sender.Send(v.Subject, v.Text(), html)
}

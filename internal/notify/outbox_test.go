package notify

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/fx"
	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

type fakeSender struct {
	fail bool
	sent int
}

func (f *fakeSender) Send(string, string, string) error {
	if f.fail {
		return errors.New("down")
	}
	f.sent++
	return nil
}

func outboxHandler(t *testing.T, channels ...Channel) *Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Handler{Store: st, FX: fx.New("USD"), Outbox: NewOutbox(st, channels...)}
}

// charge is a paid purchase, which goes to every channel.
func charge() mailInput {
	in := scenarios()["purchase"]
	in.Stats, in.Attribution = nil, nil
	return in
}

func deliverAll(o *Outbox, now time.Time) {
	for _, c := range o.Channels {
		o.deliver(c, now)
	}
}

func TestOutboxRetriesOnlyTheFailedChannel(t *testing.T) {
	for _, failing := range []string{"mail", "feishu"} {
		mail, chat := &fakeSender{fail: failing == "mail"}, &fakeSender{fail: failing == "feishu"}
		ok, bad := chat, mail
		if failing == "feishu" {
			ok, bad = mail, chat
		}
		h := outboxHandler(t, MailChannel(mail), ChatChannel("feishu", chat))
		if err := h.handle(charge()); err != nil {
			t.Fatal(err)
		}

		deliverAll(h.Outbox, time.Now())
		bad.fail = false
		deliverAll(h.Outbox, time.Now()) // the failed message waits for its retry delay
		if bad.sent != 0 {
			t.Errorf("%s failing: retried before its delay", failing)
		}
		deliverAll(h.Outbox, time.Now().Add(retryDelays[0]+5*time.Second))
		if ok.sent != 1 || bad.sent != 1 {
			t.Errorf("%s failing: working channel sent %d times, failed channel %d times after its retry", failing, ok.sent, bad.sent)
		}
	}
}

func TestOutboxGivesUpAfterTheLastRetry(t *testing.T) {
	chat := &fakeSender{fail: true}
	h := outboxHandler(t, ChatChannel("feishu", chat))
	if err := h.handle(charge()); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for range len(retryDelays) + 2 {
		deliverAll(h.Outbox, now)
		now = now.Add(48 * time.Hour)
	}
	due, err := h.Store.DueDeliveries("feishu", now, len(retryDelays)+1)
	if err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := h.Store.ReadOnly().QueryRow(`SELECT attempts FROM notification_deliveries`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 || attempts != len(retryDelays)+1 {
		t.Errorf("after giving up: %d due, %d attempts", len(due), attempts)
	}
}

func TestOutboxDuplicateNotificationAddsNoDelivery(t *testing.T) {
	mail := &fakeSender{}
	h := outboxHandler(t, MailChannel(mail))
	for range 2 {
		if err := h.handle(charge()); err != nil {
			t.Fatal(err)
		}
		deliverAll(h.Outbox, time.Now())
	}
	deliverAll(h.Outbox, time.Now().Add(48*time.Hour))
	if mail.sent != 1 {
		t.Errorf("a repeated notification was mailed %d times", mail.sent)
	}
}

func TestOutboxQueuesOnlyConfiguredChannels(t *testing.T) {
	h := outboxHandler(t, ChatChannel("feishu", &fakeSender{}))
	trial := charge()
	trial.Payload = payload("SUBSCRIBED", "INITIAL_BUY", "com.example.pawprint", "Production")
	trial.Payload.NotificationUUID = "trial"
	tx := *trial.Tx
	tx.Price, tx.OfferType = 0, 1
	trial.Tx = &tx
	for _, in := range []mailInput{charge(), trial} {
		if err := h.handle(in); err != nil {
			t.Fatal(err)
		}
	}
	var channels string
	if err := h.Store.ReadOnly().QueryRow(`SELECT group_concat(channel) FROM notification_deliveries`).Scan(&channels); err != nil {
		t.Fatal(err)
	}
	if channels != "feishu" {
		t.Errorf("queued for %q, want only the charge for feishu", channels)
	}
}

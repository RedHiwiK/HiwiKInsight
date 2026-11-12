package notify

import (
	"log/slog"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/mailer"
	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

// Channel is one way transaction messages go out. Message renders a notification for
// it; ok is false when the channel skips that notification.
type Channel struct {
	Name    string
	Sender  mailer.Sender
	Message func(v mailView) (subject, text, html string, ok bool)
}

// MailChannel sends the full transaction email for every notification.
func MailChannel(s mailer.Sender) Channel {
	return Channel{Name: "mail", Sender: s, Message: func(v mailView) (string, string, string, bool) {
		html, err := renderHTML(v)
		if err != nil {
			slog.Error("render html failed, fallback to text", "error", err)
		}
		return v.Subject, renderText(v), html, true
	}}
}

// ChatChannel sends a short message for charges, refunds and Apple's test notification.
func ChatChannel(name string, s mailer.Sender) Channel {
	return Channel{Name: name, Sender: s, Message: func(v mailView) (string, string, string, bool) {
		return v.Headline, renderChat(v), "", v.ToChat
	}}
}

// retryDelays are the waits after each failed attempt; after the last one the message
// is given up and its last error stays in notification_deliveries.
var retryDelays = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 24 * time.Hour}

// Outbox delivers stored transaction messages. Each channel has its own loop and retries,
// so a channel that fails never makes another one send twice.
type Outbox struct {
	Store    *store.Store
	Channels []Channel
	wake     map[string]chan struct{}
}

func NewOutbox(st *store.Store, channels ...Channel) *Outbox {
	o := &Outbox{Store: st, Channels: channels, wake: map[string]chan struct{}{}}
	for _, c := range channels {
		o.wake[c.Name] = make(chan struct{}, 1)
	}
	return o
}

// Loop starts one sender per channel. Messages left over from before a restart go out first.
func (o *Outbox) Loop() {
	for _, c := range o.Channels {
		go func() {
			for {
				o.deliver(c, time.Now())
				select {
				case <-o.wake[c.Name]:
				case <-time.After(time.Minute):
				}
			}
		}()
	}
}

// Wake makes every channel look for due messages now instead of at its next minute.
func (o *Outbox) Wake() {
	for _, w := range o.wake {
		select {
		case w <- struct{}{}:
		default:
		}
	}
}

// deliver sends a channel's due messages once each.
func (o *Outbox) deliver(c Channel, now time.Time) {
	due, err := o.Store.DueDeliveries(c.Name, now, len(retryDelays)+1)
	if err != nil {
		slog.Error("load deliveries failed", "channel", c.Name, "error", err)
		return
	}
	for _, d := range due {
		if sendErr := c.Sender.Send(d.Subject, d.Text, d.HTML); sendErr == nil {
			err = o.Store.MarkDelivered(d, time.Now())
		} else if d.Attempts < len(retryDelays) {
			slog.Warn("delivery failed, will retry", "channel", c.Name, "notificationUUID", d.NotificationUUID,
				"attempt", d.Attempts+1, "error", sendErr)
			err = o.Store.MarkFailed(d, sendErr, time.Now().Add(retryDelays[d.Attempts]))
		} else {
			slog.Error("delivery given up", "channel", c.Name, "notificationUUID", d.NotificationUUID,
				"attempts", d.Attempts+1, "error", sendErr)
			err = o.Store.MarkFailed(d, sendErr, time.Now())
		}
		if err != nil {
			slog.Error("update delivery failed", "channel", c.Name, "notificationUUID", d.NotificationUUID, "error", err)
		}
	}
}

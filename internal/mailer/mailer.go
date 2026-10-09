// Package mailer sends multipart/alternative (text + HTML) emails over SMTP.
// Port 465 uses implicit TLS; any other port (usually 587) upgrades with STARTTLS.
package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Sender sends one email to the configured recipients. htmlBody may be empty.
type Sender interface {
	Send(subject, textBody, htmlBody string) error
}

// LogSender is used when SMTP is not configured: emails are only logged, so a server
// can be deployed first and mail set up later.
type LogSender struct{}

func (LogSender) Send(subject, _, _ string) error {
	slog.Info("mail skipped (SMTP not configured)", "subject", subject)
	return nil
}

type SMTPMailer struct {
	Host     string
	Port     int
	Username string
	Password string // many providers require an app-specific password here
	From     string // sender address; defaults to Username
	To       []string
}

func (m *SMTPMailer) from() string {
	if m.From != "" {
		return m.From
	}
	return m.Username
}

func (m *SMTPMailer) dial() (*smtp.Client, error) {
	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if m.Port == 465 {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: m.Host})
		if err != nil {
			return nil, fmt.Errorf("smtp dial: %w", err)
		}
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		c, err := smtp.NewClient(conn, m.Host)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("smtp client: %w", err)
		}
		return c, nil
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("smtp client: %w", err)
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: m.Host}); err != nil {
			c.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	}
	return c, nil
}

// Send sends the email; only plain text is sent when htmlBody is empty.
func (m *SMTPMailer) Send(subject, textBody, htmlBody string) error {
	c, err := m.dial()
	if err != nil {
		return err
	}
	defer c.Close()

	if m.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(m.from()); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, to := range m.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", to, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(m.buildMessage(subject, textBody, htmlBody)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return c.Quit()
}

func (m *SMTPMailer) buildMessage(subject, textBody, htmlBody string) []byte {
	from := m.from()
	domain := from[strings.LastIndex(from, "@")+1:]
	now := time.Now()
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s <%s>\r\n", mime.BEncoding.Encode("UTF-8", "HiwiKInsight"), from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.BEncoding.Encode("UTF-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%d@%s>\r\n", now.UnixNano(), domain)
	b.WriteString("MIME-Version: 1.0\r\n")

	if htmlBody == "" {
		writePart(&b, "text/plain", textBody)
		return []byte(b.String())
	}

	boundary := fmt.Sprintf("hiwikinsight-%d", now.UnixNano())
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	writePart(&b, "text/plain", textBody)
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	writePart(&b, "text/html", htmlBody)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String())
}

func writePart(b *strings.Builder, contentType, body string) {
	fmt.Fprintf(b, "Content-Type: %s; charset=UTF-8\r\n", contentType)
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for len(encoded) > 76 {
		b.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded + "\r\n")
}

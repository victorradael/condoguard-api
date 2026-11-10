// Package mailer sends the system's e-mails (password recovery, e-mail
// verification). In the local environment they go to Mailpit over SMTP.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// Message is a plain-text e-mail.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// ErrInvalidMessage is returned for a message that could be used for header injection or has no recipient.
var ErrInvalidMessage = errors.New("mailer: invalid message")

func (m Message) validate() (to string, err error) {
	addr, perr := mail.ParseAddress(m.To)
	if perr != nil || strings.ContainsAny(m.To, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") {
		return "", ErrInvalidMessage
	}
	return addr.Address, nil
}

// ── SMTP ──────────────────────────────────────────────────────────────────────

// SMTP sends through an SMTP server. Mailpit needs no authentication; with a
// username the connection authenticates (and net/smtp upgrades to TLS when the
// server offers STARTTLS).
type SMTP struct {
	Addr     string // host:port
	From     string // address of the sender
	Username string
	Password string
	Timeout  time.Duration
}

// Send delivers the message or fails after Timeout (default 10s).
func (s *SMTP) Send(ctx context.Context, m Message) error {
	to, err := m.validate()
	if err != nil {
		return err
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("mailer: invalid From %q: %w", s.From, err)
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("mailer: invalid address %q: %w", s.Addr, err)
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("mailer: dial: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mailer: smtp: %w", err)
	}
	defer func() { _ = c.Close() }()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err = c.StartTLS(tlsConfig(host)); err != nil {
			return fmt.Errorf("mailer: starttls: %w", err)
		}
	}
	if s.Username != "" {
		if err = c.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}
	if err = c.Mail(from.Address); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	if err = c.Rcpt(to); err != nil {
		return fmt.Errorf("mailer: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err = w.Write(render(from.String(), to, m)); err != nil {
		return fmt.Errorf("mailer: write: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("mailer: send: %w", err)
	}
	return c.Quit()
}

func render(from, to string, m Message) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n"))
	b.WriteString("\r\n")
	return []byte(b.String())
}

// ── In memory (tests and dev without SMTP) ────────────────────────────────────

// Memory keeps the messages instead of sending them.
type Memory struct {
	mu   sync.Mutex
	sent []Message
}

// Send records the message.
func (m *Memory) Send(_ context.Context, msg Message) error {
	if _, err := msg.validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

// Sent returns a copy of the messages sent so far.
func (m *Memory) Sent() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.sent...)
}

// To returns the messages sent to the address, oldest first.
func (m *Memory) To(addr string) []Message {
	var out []Message
	for _, msg := range m.Sent() {
		if strings.EqualFold(msg.To, addr) {
			out = append(out, msg)
		}
	}
	return out
}

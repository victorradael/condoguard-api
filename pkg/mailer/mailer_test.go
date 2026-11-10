package mailer_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/pkg/mailer"
)

// fakeSMTP speaks just enough SMTP to capture one message.
func fakeSMTP(t *testing.T) (addr string, got <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	out := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		w("220 fake ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case inData && line == ".":
				inData = false
				out <- data.String()
				w("250 ok")
			case inData:
				data.WriteString(line + "\n")
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				w("250 fake")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				w("354 go")
			case strings.HasPrefix(line, "QUIT"):
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	return ln.Addr().String(), out
}

func TestSMTP_Send(t *testing.T) {
	addr, got := fakeSMTP(t)
	s := &mailer.SMTP{Addr: addr, From: "Condoguard <no-reply@condoguard.local>"}
	err := s.Send(context.Background(), mailer.Message{To: "ana@exemplo.test", Subject: "Redefinição de senha — Condoguard", Body: "Olá\nlink"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		for _, want := range []string{"From: \"Condoguard\" <no-reply@condoguard.local>", "To: ana@exemplo.test", "Subject: =?utf-8?q?", "text/plain; charset=utf-8", "Olá", "link"} {
			if !strings.Contains(msg, want) {
				t.Errorf("message lacks %q:\n%s", want, msg)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message received")
	}
}

func TestSend_RejectsHeaderInjection(t *testing.T) {
	m := &mailer.Memory{}
	for _, msg := range []mailer.Message{
		{To: "ana@exemplo.test\r\nBcc: x@y.z", Subject: "s"},
		{To: "ana@exemplo.test", Subject: "s\r\nBcc: x@y.z"},
		{To: "not an address", Subject: "s"},
		{To: "", Subject: "s"},
	} {
		if err := m.Send(context.Background(), msg); !errors.Is(err, mailer.ErrInvalidMessage) {
			t.Errorf("%+v: expected ErrInvalidMessage, got %v", msg, err)
		}
		s := &mailer.SMTP{Addr: "127.0.0.1:1", From: "a@b.c"}
		if err := s.Send(context.Background(), msg); !errors.Is(err, mailer.ErrInvalidMessage) {
			t.Errorf("SMTP %+v: expected ErrInvalidMessage before dialing, got %v", msg, err)
		}
	}
	if len(m.Sent()) != 0 {
		t.Error("nothing may be sent")
	}
}

func TestSMTP_ServerDown(t *testing.T) {
	s := &mailer.SMTP{Addr: "127.0.0.1:1", From: "a@b.c", Timeout: time.Second}
	if err := s.Send(context.Background(), mailer.Message{To: "ana@exemplo.test", Subject: "s"}); err == nil {
		t.Error("expected an error")
	}
}

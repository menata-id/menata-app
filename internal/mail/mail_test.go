package mail

import (
	"context"
	"strings"
	"testing"

	"menata.app/internal/config"
)

func TestNewMailerFromConfig_noHostGivesLogMailer(t *testing.T) {
	m := NewMailerFromConfig(config.Config{})
	if _, ok := m.(LogMailer); !ok {
		t.Errorf("NewMailerFromConfig(no SMTP host) = %T, want LogMailer", m)
	}
}

func TestNewMailerFromConfig_hostGivesSMTPMailer(t *testing.T) {
	m := NewMailerFromConfig(config.Config{SMTPHost: "smtp.example.com", SMTPPort: "587"})
	smtpMailer, ok := m.(SMTPMailer)
	if !ok {
		t.Fatalf("NewMailerFromConfig(SMTP host set) = %T, want SMTPMailer", m)
	}
	if smtpMailer.Host != "smtp.example.com" || smtpMailer.Port != "587" {
		t.Errorf("SMTPMailer = %+v, want Host=smtp.example.com Port=587", smtpMailer)
	}
}

func TestLogMailer_send(t *testing.T) {
	if err := (LogMailer{}).Send(context.Background(), "person@example.com", "Subject", "Body"); err != nil {
		t.Errorf("LogMailer.Send() error = %v, want nil", err)
	}
}

// TestSMTPMailer_rejectsCRLF is the regression test for a CodeQL go/email-injection finding
// (2026-09-19): to/subject/from must be rejected before Send ever reaches the network, on both
// the sendImplicitTLS (port 465) and net/smtp.SendMail (any other port) paths -- constructing an
// SMTPMailer with an unreachable host/port and confirming Send still returns quickly with a
// validation error (not a dial/timeout error) is what proves the check runs first.
func TestSMTPMailer_rejectsCRLF(t *testing.T) {
	const injected = "victim@example.com\r\nBcc: attacker@evil.com"
	cases := []struct {
		name, to, subject, from string
	}{
		{"CRLF in to", injected, "Subject", "from@example.com"},
		{"CRLF in subject", "victim@example.com", "Subject\r\nBcc: attacker@evil.com", "from@example.com"},
		{"CRLF in from", "victim@example.com", "Subject", injected},
		{"bare LF", "victim@example.com\nBcc: attacker@evil.com", "Subject", "from@example.com"},
	}
	for _, port := range []string{"587", "465"} {
		for _, c := range cases {
			t.Run(port+"/"+c.name, func(t *testing.T) {
				m := SMTPMailer{Host: "unreachable.invalid", Port: port, From: c.from}
				err := m.Send(context.Background(), c.to, c.subject, "body")
				if err == nil {
					t.Fatal("Send() error = nil, want a validation error")
				}
				if strings.Contains(err.Error(), "dial") || strings.Contains(err.Error(), "lookup") || strings.Contains(err.Error(), "tls") {
					t.Errorf("Send() error = %v, looks like a network error -- validation must reject before any connection attempt", err)
				}
			})
		}
	}
}

func TestSMTPMailer_acceptsCleanValues(t *testing.T) {
	if err := validateNoCRLF("victim@example.com", "to"); err != nil {
		t.Errorf("validateNoCRLF(clean value) error = %v, want nil", err)
	}
}

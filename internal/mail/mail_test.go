package mail

import (
	"context"
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

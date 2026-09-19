// Package mail sends outbound email (verification links, password resets) -- a pluggable Mailer
// so the rest of the app never depends on which transport is behind it. It carries no business or
// Runtime Metadata concerns and never touches the database or the renderer: it sends bytes to an
// address it's given, nothing more.
package mail

import (
	"context"
	"fmt"
	"log"
	"net/smtp"

	"menata.app/internal/config"
)

// Mailer sends one email. Implementations must not block indefinitely -- callers in a request path
// expect this to return promptly.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// NewMailerFromConfig returns an SMTPMailer if cfg declares a real SMTP host, else a LogMailer.
// Never guesses at credentials that were never supplied.
func NewMailerFromConfig(cfg config.Config) Mailer {
	if cfg.SMTPHost == "" {
		return LogMailer{}
	}
	return SMTPMailer{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}
}

// SMTPMailer sends real email via net/smtp.SendMail -- stdlib only (007 §4.10's single-binary
// posture), no SDK, no external service dependency beyond the SMTP relay itself. SendMail issues
// STARTTLS automatically when the server advertises it (true for essentially every real relay on
// port 587), which covers the common case without this type needing to manage TLS itself.
type SMTPMailer struct {
	Host, Port, Username, Password, From string
}

func (m SMTPMailer) Send(_ context.Context, to, subject, body string) error {
	addr := m.Host + ":" + m.Port
	auth := smtp.PlainAuth("", m.Username, m.Password, m.Host)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.From, to, subject, body)
	if err := smtp.SendMail(addr, auth, m.From, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("send email to %s: %w", to, err)
	}
	return nil
}

// LogMailer writes the email to the server log instead of sending it. Not a placeholder to delete
// once real SMTP exists: it is what keeps every email-driven flow testable without real SMTP
// configured, and a real fallback if SMTP is ever misconfigured in production -- a verification or
// reset email that can't be sent should be visible somewhere, never silently dropped.
type LogMailer struct{}

func (LogMailer) Send(_ context.Context, to, subject, body string) error {
	log.Printf("[mail] to=%s subject=%q\n%s", to, subject, body)
	return nil
}

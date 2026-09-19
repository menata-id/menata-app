// Package mail sends outbound email (verification links, password resets) -- a pluggable Mailer
// so the rest of the app never depends on which transport is behind it. It carries no business or
// Runtime Metadata concerns and never touches the database or the renderer: it sends bytes to an
// address it's given, nothing more.
package mail

import (
	"context"
	"crypto/tls"
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

// SMTPMailer sends real email -- stdlib only (007 §4.10's single-binary posture), no SDK, no
// external service dependency beyond the SMTP relay itself. Port 465 is dialed with implicit TLS
// from the first byte (the historical SMTPS convention, still what most real mailbox providers'
// own "outgoing server" settings list -- e.g. Hostinger's own smtp.hostinger.com:465/SSL); any
// other port goes through net/smtp.SendMail, which negotiates STARTTLS on a plaintext connection
// instead (port 587's own convention). These are two different handshakes, not one mechanism with
// two port numbers -- net/smtp.SendMail alone only ever speaks the second one, so a 465 relay
// needs its own path.
type SMTPMailer struct {
	Host, Port, Username, Password, From string
}

func (m SMTPMailer) Send(_ context.Context, to, subject, body string) error {
	addr := m.Host + ":" + m.Port
	auth := smtp.PlainAuth("", m.Username, m.Password, m.Host)
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.From, to, subject, body))

	var err error
	if m.Port == "465" {
		err = m.sendImplicitTLS(addr, auth, to, msg)
	} else {
		err = smtp.SendMail(addr, auth, m.From, []string{to}, msg)
	}
	if err != nil {
		return fmt.Errorf("send email to %s: %w", to, err)
	}
	return nil
}

// sendImplicitTLS speaks SMTP over a connection that is already TLS from the start (port 465),
// which net/smtp.SendMail cannot do on its own -- it only ever dials plaintext and optionally
// upgrades via STARTTLS afterward.
func (m SMTPMailer) sendImplicitTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: m.Host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := client.Mail(m.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close message writer: %w", err)
	}
	return client.Quit()
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

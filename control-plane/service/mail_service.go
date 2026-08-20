package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/config"
)

const smtpDialTimeout = 30 * time.Second

// ErrMailDisabled คืนเมื่อไม่ได้ตั้ง ORYCA_API_SMTP_HOST (ระบบปิดการส่งเมล)
var ErrMailDisabled = errors.New("mail is disabled: ORYCA_API_SMTP_HOST is not set")

type MailService struct {
	cfg *config.SMTPConfig
}

func NewMailService(cfg *config.SMTPConfig) *MailService {
	return &MailService{cfg: cfg}
}

// SendTemplate renders an embedded email template with variables and sends it via the configured SMTP server.
// vars keys match template placeholders e.g. "name" replaces "{{name}}" in html.
func (s *MailService) SendTemplate(ctx context.Context, toEmail, alias string, vars map[string]string) error {
	if s.cfg.Host == "" {
		return ErrMailDisabled
	}

	tmpl, err := getMailTemplate(alias)
	if err != nil {
		return fmt.Errorf("mail: get template %q: %w", alias, err)
	}

	body := renderVariables(tmpl.Html, vars)

	senderName := s.cfg.SenderName
	if senderName == "" {
		senderName = "Oryca"
	}
	senderEmail := s.cfg.SenderEmail
	if senderEmail == "" {
		senderEmail = s.cfg.User
	}
	from := mail.Address{Name: senderName, Address: senderEmail}

	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from.String()))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", toEmail))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", tmpl.Subject))
	msg.WriteString("MIME-version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := s.cfg.Host + ":" + s.cfg.Port

	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return fmt.Errorf("mail: connect to %s: %w", addr, err)
	}
	conn.Close()

	if !s.cfg.Auth {
		return smtpSendNoAuth(addr, from.Address, toEmail, msg.Bytes())
	}

	return smtpSendWithAuth(addr, s.cfg.Host, s.cfg.User, s.cfg.Password, from.Address, toEmail, msg.Bytes(), s.cfg.TlsSkipVerify)
}

func renderVariables(htmlBody string, vars map[string]string) string {
	for k, v := range vars {
		htmlBody = strings.ReplaceAll(htmlBody, "{{"+k+"}}", v)
	}
	return htmlBody
}

func smtpSendWithAuth(addr, host, user, password, from, to string, msg []byte, skipVerify bool) error {
	tlsCfg := &tls.Config{ServerName: host, InsecureSkipVerify: skipVerify}

	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("mail: dial %s: %w", addr, err)
	}
	defer c.Close()

	if err := c.StartTLS(tlsCfg); err != nil {
		return fmt.Errorf("mail: STARTTLS: %w", err)
	}

	auth := smtp.PlainAuth("", user, password, host)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("mail: auth: %w", err)
	}

	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}

	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return c.Quit()
}

func smtpSendNoAuth(addr, from, to string, msg []byte) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}

	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return c.Quit()
}

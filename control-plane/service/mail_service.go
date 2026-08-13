package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/model"
)

const smtpDialTimeout = 30 * time.Second

type mailServerDefaultGetter interface {
	GetDefault(ctx context.Context) (*model.MailServer, error)
}

type emailTemplateAliasGetter interface {
	GetByAlias(ctx context.Context, alias string) (*model.EmailTemplate, error)
}

type MailService struct {
	mailServerSvc    mailServerDefaultGetter
	emailTemplateSvc emailTemplateAliasGetter
}

func NewMailService(mailServerSvc mailServerDefaultGetter, emailTemplateSvc emailTemplateAliasGetter) *MailService {
	return &MailService{
		mailServerSvc:    mailServerSvc,
		emailTemplateSvc: emailTemplateSvc,
	}
}

// SendTemplate renders an email template with variables and sends it via the default SMTP server.
// vars keys match template placeholders e.g. "name" replaces "{{name}}" in htmlBody.
func (s *MailService) SendTemplate(ctx context.Context, toEmail, alias string, vars map[string]string) error {
	ms, err := s.mailServerSvc.GetDefault(ctx)
	if err != nil {
		return fmt.Errorf("mail: get default mail server: %w", err)
	}

	tmpl, err := s.emailTemplateSvc.GetByAlias(ctx, alias)
	if err != nil {
		return fmt.Errorf("mail: get template %q: %w", alias, err)
	}

	body := renderVariables(tmpl.HtmlBody, vars)

	senderName := ms.Sender
	if senderName == "" {
		senderName = "Oryca"
	}
	senderEmail := ms.SenderEmail
	if senderEmail == "" {
		senderEmail = ms.Smtp.User
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

	addr := ms.Smtp.Host + ":" + ms.Smtp.Port

	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return fmt.Errorf("mail: connect to %s: %w", addr, err)
	}
	conn.Close()

	skipVerify := ms.Smtp.TlsSkipVerify != nil && *ms.Smtp.TlsSkipVerify

	if ms.Auth != nil && !*ms.Auth {
		return smtpSendNoAuth(addr, from.Address, toEmail, msg.Bytes())
	}

	return smtpSendWithAuth(addr, ms.Smtp.Host, ms.Smtp.User, ms.Smtp.Password, from.Address, toEmail, msg.Bytes(), skipVerify)
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

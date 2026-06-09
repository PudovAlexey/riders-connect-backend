// Package mailer is the single SMTP transport used across the app (auth login
// codes, chat/event notifications). The dial logic is intentionally identical to
// what auth used before — bounded timeouts plus plain "tcp" so happy-eyeballs can
// reach the provider over IPv6 (the hoster blocks outbound IPv4 SMTP).
package mailer

import (
	"crypto/tls"
	"fmt"
	"log"
	"mime"
	"net"
	"net/smtp"
	"time"

	"riders-connect/internal/config"
)

type Mailer struct {
	host string
	port string
	from string
	pass string
	dev  bool
}

func New(cfg *config.Config) *Mailer {
	return &Mailer{
		host: cfg.SMTPHost,
		port: cfg.SMTPPort,
		from: cfg.SMTPFrom,
		pass: cfg.SMTPPass,
		// No real sender configured (or dev env) → don't talk to SMTP, just log.
		dev: cfg.AppEnv == "development" || cfg.SMTPFrom == "",
	}
}

// Send delivers a plain-text (UTF-8) email. It is best-effort and synchronous;
// callers that don't want to block should invoke it in a goroutine (as auth and
// the notification hooks do).
func (m *Mailer) Send(to, subject, body string) error {
	if m.dev {
		log.Printf("[DEV] email -> %s | %s\n%s", to, subject, body)
		return nil
	}

	addr := m.host + ":" + m.port
	// Encode the Subject so non-ASCII (Cyrillic) renders correctly in clients.
	encSubject := mime.QEncoding.Encode("utf-8", subject)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		m.from, to, encSubject, body,
	)

	// Bounded dial + overall deadline, чтобы зависший SMTP (напр. egress-блок порта
	// у хостера) не вешал запрос навсегда и не плодил горутины.
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsCfg := &tls.Config{ServerName: m.host}

	// Провайдер блокирует исходящий IPv4 SMTP (587/465 timeout) — рабочий путь только IPv6.
	// Контейнеру включён IPv6 (docker-compose: enable_ipv6), "tcp" даёт happy-eyeballs и
	// уходит к smtp.yandex.ru по IPv6.
	var conn net.Conn
	var err error
	if m.port == "465" {
		// Implicit TLS (SSL) — Yandex и др. на 465.
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		// Plaintext + STARTTLS — Gmail/Yandex на 587.
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer c.Close()

	if m.port != "465" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return err
			}
		}
	}

	auth := smtp.PlainAuth("", m.from, m.pass, m.host)
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(m.from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write([]byte(msg)); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Package mailer is the single email transport used across the app (auth login
// codes, chat/event notifications).
//
// Transports, picked at construction in priority order:
//   - HTTP API (Mailopost) when MAILOPOST_TOKEN is set — a plain HTTPS POST with a
//     Bearer token. Sends to arbitrary recipients, no per-recipient setup.
//   - HTTP API (Brevo) when EMAIL_API_KEY is set — a plain HTTPS POST.
//   - SMTP otherwise — kept as a fallback / for environments where it works. The
//     dial uses plain "tcp" so happy-eyeballs can reach the provider over IPv6.
//
// The HTTP transports are the production path: the hoster blocks outbound SMTP
// (25/465/587/2525) and the IPv6/NAT66 workaround keeps dying (the VM has no
// global IPv6), so SMTP from this server is unreliable. HTTPS egress works fine.
package mailer

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"time"

	"riders-connect/internal/config"
)

// brevoEndpoint is Brevo's transactional email API. Auth is the api-key header.
const brevoEndpoint = "https://api.brevo.com/v3/smtp/email"

// mailopostEndpoint is Mailopost's transactional email API. Auth is a Bearer token.
const mailopostEndpoint = "https://api.mailopost.ru/v1/email/messages"

type Mailer struct {
	host           string
	port           string
	from           string
	pass           string
	apiKey         string // Brevo HTTP API key; when set, Brevo HTTP transport is used.
	mailopostToken string // Mailopost Bearer token; takes priority when set.
	fromName       string
	dev            bool
	http           *http.Client
}

func New(cfg *config.Config) *Mailer {
	return &Mailer{
		host:           cfg.SMTPHost,
		port:           cfg.SMTPPort,
		from:           cfg.SMTPFrom,
		pass:           cfg.SMTPPass,
		apiKey:         cfg.EmailAPIKey,
		mailopostToken: cfg.MailopostToken,
		fromName:       cfg.EmailFromName,
		// No real sender configured (or dev env) → don't talk to any provider,
		// just log. A "from" address is required for both transports.
		dev:  cfg.AppEnv == "development" || cfg.SMTPFrom == "",
		http: &http.Client{Timeout: 20 * time.Second},
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
	if m.mailopostToken != "" {
		return m.sendMailopost(to, subject, body)
	}
	if m.apiKey != "" {
		return m.sendHTTP(to, subject, body)
	}
	return m.sendSMTP(to, subject, body)
}

// sendMailopost posts the message to Mailopost's transactional API over HTTPS.
// Like the Brevo path, no SMTP and no IPv6 dependency. Auth is a Bearer token;
// from_email must be on a domain verified in the Mailopost account.
func (m *Mailer) sendMailopost(to, subject, body string) error {
	fromName := m.fromName
	if fromName == "" {
		fromName = "Riders Connect"
	}
	payload := map[string]any{
		"from_email": m.from,
		"from_name":  fromName,
		"to":         to,
		"subject":    subject,
		"text":       body,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, mailopostEndpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.mailopostToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		// Surface Mailopost's error body (bad token, unverified domain, quota) so
		// the auth log line is actionable. Cap it so a huge body can't flood logs.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("mailopost send failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

// sendHTTP posts the message to Brevo's transactional API over HTTPS. No SMTP,
// no IPv6 dependency — this is the production transport.
func (m *Mailer) sendHTTP(to, subject, body string) error {
	fromName := m.fromName
	if fromName == "" {
		fromName = "Riders Connect"
	}
	payload := map[string]any{
		"sender":      map[string]string{"email": m.from, "name": fromName},
		"to":          []map[string]string{{"email": to}},
		"subject":     subject,
		"textContent": body,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, brevoEndpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		// Surface Brevo's error body (bad key, unverified sender, quota) so the
		// auth log line is actionable. Cap it so a huge body can't flood logs.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("brevo send failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

// sendSMTP is the legacy transport. Kept as a fallback; see the package doc for
// why it's unreliable on the production host.
func (m *Mailer) sendSMTP(to, subject, body string) error {
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

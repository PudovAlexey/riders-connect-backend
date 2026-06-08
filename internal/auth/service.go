package auth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/smtp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"riders-connect/internal/config"
	"riders-connect/internal/models"
)

var ErrInvalidCode = errors.New("invalid or expired code")

type Service struct {
	repo *Repository
	cfg  *config.Config
}

func NewService(repo *Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

func (s *Service) SendCode(ctx context.Context, email string) error {
	code, err := randomCode(6)
	if err != nil {
		return err
	}
	if err := s.repo.SaveCode(ctx, email, code, time.Now().Add(10*time.Minute)); err != nil {
		return err
	}
	// Код уже сохранён в БД — клиенту незачем ждать доставки письма.
	// Отправляем асинхронно, чтобы медленный SMTP не держал HTTP-ответ.
	go func() {
		if err := s.sendEmail(email, code); err != nil {
			log.Printf("auth: failed to send verification code to %s: %v", email, err)
		}
	}()
	return nil
}

func (s *Service) Verify(ctx context.Context, email, code string) (string, *models.User, error) {
	ok, err := s.repo.VerifyCode(ctx, email, code)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, ErrInvalidCode
	}
	user, err := s.repo.UpsertUser(ctx, email)
	if err != nil {
		return "", nil, err
	}
	token, err := s.makeToken(user)
	return token, user, err
}

func (s *Service) makeToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"sub": user.ID.String(),
		"exp": time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
}

func (s *Service) sendEmail(to, code string) error {
	if s.cfg.AppEnv == "development" || s.cfg.SMTPFrom == "" {
		fmt.Printf("[DEV] verification code for %s: %s\n", to, code)
		return nil
	}

	addr := s.cfg.SMTPHost + ":" + s.cfg.SMTPPort
	body := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Your verification code\r\n\r\nYour code: %s\r\n",
		s.cfg.SMTPFrom, to, code,
	)

	// Bounded dial + overall deadline, чтобы зависший SMTP (напр. egress-блок порта
	// у хостера) не вешал запрос навсегда и не плодил горутины.
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsCfg := &tls.Config{ServerName: s.cfg.SMTPHost}

	// Форсим IPv4 ("tcp4"): контейнер на дефолтном Docker-bridge не имеет IPv6-маршрута,
	// а smtp.yandex.ru — dual-stack. Без этого dial уходил на AAAA-адрес и висел.
	var conn net.Conn
	var err error
	if s.cfg.SMTPPort == "465" {
		// Implicit TLS (SSL) — Yandex и др. на 465.
		conn, err = tls.DialWithDialer(dialer, "tcp4", addr, tlsCfg)
	} else {
		// Plaintext + STARTTLS — Gmail/Yandex на 587.
		conn, err = dialer.Dial("tcp4", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	c, err := smtp.NewClient(conn, s.cfg.SMTPHost)
	if err != nil {
		return err
	}
	defer c.Close()

	if s.cfg.SMTPPort != "465" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return err
			}
		}
	}

	auth := smtp.PlainAuth("", s.cfg.SMTPFrom, s.cfg.SMTPPass, s.cfg.SMTPHost)
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(s.cfg.SMTPFrom); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write([]byte(body)); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func randomCode(n int) (string, error) {
	const digits = "0123456789"
	result := make([]byte, n)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		result[i] = digits[num.Int64()]
	}
	return string(result), nil
}

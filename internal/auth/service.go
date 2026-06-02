package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
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
	return s.sendEmail(email, code)
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
	auth := smtp.PlainAuth("", s.cfg.SMTPFrom, s.cfg.SMTPPass, s.cfg.SMTPHost)
	body := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Your verification code\r\n\r\nYour code: %s\r\n",
		s.cfg.SMTPFrom, to, code,
	)
	return smtp.SendMail(
		s.cfg.SMTPHost+":"+s.cfg.SMTPPort,
		auth, s.cfg.SMTPFrom, []string{to}, []byte(body),
	)
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

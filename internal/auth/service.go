package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"log"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"riders-connect/internal/config"
	"riders-connect/internal/mailer"
	"riders-connect/internal/models"
)

var ErrInvalidCode = errors.New("invalid or expired code")

type Service struct {
	repo *Repository
	cfg  *config.Config
	mail *mailer.Mailer
}

func NewService(repo *Repository, cfg *config.Config, mail *mailer.Mailer) *Service {
	return &Service{repo: repo, cfg: cfg, mail: mail}
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
	// TEMPORARY master-code bypass (AUTH_MASTER_CODE). When set, this code is
	// accepted for ANY email so logins keep working while email delivery is down.
	// This is a FULL AUTH BYPASS — keep the env var set only during the outage and
	// remove it the moment a real mail transport works. Constant-time compare so
	// the code can't be guessed via timing; each use is logged for an audit trail.
	if m := s.cfg.AuthMasterCode; m != "" && subtle.ConstantTimeCompare([]byte(code), []byte(m)) == 1 {
		log.Printf("auth: MASTER CODE bypass used for %s", email)
	} else {
		ok, err := s.repo.VerifyCode(ctx, email, code)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			return "", nil, ErrInvalidCode
		}
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
	return s.mail.Send(to, "Your verification code", "Your code: "+code)
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

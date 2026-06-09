package push

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
)

type Service struct {
	repo    *Repository
	public  string
	private string
	subject string
}

func NewService(repo *Repository, vapidPublic, vapidPrivate, vapidSubject string) *Service {
	return &Service{repo: repo, public: vapidPublic, private: vapidPrivate, subject: vapidSubject}
}

// enabled reports whether VAPID keys are configured; if not, SendToUser is a
// no-op so callers cleanly fall back to email.
func (s *Service) enabled() bool { return s.private != "" && s.public != "" }

func (s *Service) PublicKey() string { return s.public }

func (s *Service) Save(ctx context.Context, userID uuid.UUID, endpoint, p256dh, auth string) error {
	return s.repo.Upsert(ctx, userID, endpoint, p256dh, auth)
}

func (s *Service) Delete(ctx context.Context, endpoint string) error {
	return s.repo.DeleteByEndpoint(ctx, endpoint)
}

// payload is the JSON the service worker receives in its `push` event.
type payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// SendToUser delivers a push to every subscription of a user and returns how
// many were delivered (0 if push is disabled or the user has no subscriptions).
// Dead subscriptions (404/410) are pruned. Best-effort: errors are logged.
func (s *Service) SendToUser(ctx context.Context, userID uuid.UUID, title, body, url, tag string) int {
	if !s.enabled() {
		return 0
	}
	subs, err := s.repo.ListByUserID(ctx, userID)
	if err != nil || len(subs) == 0 {
		return 0
	}
	msg, err := json.Marshal(payload{Title: title, Body: body, URL: url, Tag: tag})
	if err != nil {
		return 0
	}

	delivered := 0
	for _, sub := range subs {
		resp, err := webpush.SendNotificationWithContext(ctx, msg, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		}, &webpush.Options{
			Subscriber:      s.subject,
			VAPIDPublicKey:  s.public,
			VAPIDPrivateKey: s.private,
			TTL:             60,
		})
		if err != nil {
			log.Printf("push: send to %s: %v", userID, err)
			continue
		}
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			resp.Body.Close()
			delivered++
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			resp.Body.Close()
			// Subscription is dead — drop it so we stop trying.
			dctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = s.repo.DeleteByEndpoint(dctx, sub.Endpoint)
			cancel()
		default:
			// Log the push service's error body (e.g. Apple's reason) to diagnose.
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			log.Printf("push: send to %s: status %d: %s", userID, resp.StatusCode, string(body))
		}
	}
	return delivered
}

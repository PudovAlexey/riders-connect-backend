package main

import (
	"crypto/elliptic"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"riders-connect/internal/auth"
	"riders-connect/internal/chat"
	"riders-connect/internal/config"
	"riders-connect/internal/contacts"
	"riders-connect/internal/database"
	"riders-connect/internal/events"
	"riders-connect/internal/garage"
	"riders-connect/internal/geo"
	"riders-connect/internal/mailer"
	"riders-connect/internal/media"
	"riders-connect/internal/middleware"
	"riders-connect/internal/points"
	"riders-connect/internal/profile"
	"riders-connect/internal/push"
	"riders-connect/internal/reviews"
	"riders-connect/internal/routes"
)

// vapidDecode decodes a base64url VAPID key, tolerating padded or raw form
// (same logic webpush-go uses internally).
func vapidDecode(key string) ([]byte, error) {
	if b, err := base64.URLEncoding.DecodeString(key); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(key)
}

func main() {
	// -genvapid prints a fresh VAPID keypair (as env lines) and exits. Used by
	// deploy/up.sh to seed deploy/.env on first run.
	genVAPID := flag.Bool("genvapid", false, "generate a VAPID keypair and exit")
	checkVAPID := flag.Bool("checkvapid", false, "verify the configured VAPID keypair matches and exit")
	flag.Parse()
	if *genVAPID {
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			log.Fatalf("genvapid: %v", err)
		}
		fmt.Printf("VAPID_PUBLIC=%s\nVAPID_PRIVATE=%s\n", pub, priv)
		return
	}
	if *checkVAPID {
		c := config.Load()
		priv, err := vapidDecode(c.VAPIDPrivate)
		if err != nil {
			log.Fatalf("checkvapid: bad VAPID_PRIVATE: %v", err)
		}
		x, y := elliptic.P256().ScalarBaseMult(priv)
		derived := base64.RawURLEncoding.EncodeToString(elliptic.Marshal(elliptic.P256(), x, y))
		fmt.Printf("VAPID_PUBLIC (configured): %s\n", c.VAPIDPublic)
		fmt.Printf("VAPID_PUBLIC (derived)   : %s\n", derived)
		fmt.Printf("VAPID_SUBJECT            : %s\n", c.VAPIDSubject)
		if derived == c.VAPIDPublic {
			fmt.Println("RESULT: MATCH ✓")
		} else {
			fmt.Println("RESULT: MISMATCH ✗  (regenerate keys)")
		}
		return
	}

	cfg := config.Load()

	db := database.Connect(cfg.DatabaseURL)
	defer db.Close()

	database.RunMigrations(db)

	// Repositories
	authRepo := auth.NewRepository(db)
	profileRepo := profile.NewRepository(db)
	geoRepo := geo.NewRepository(db)
	chatRepo := chat.NewRepository(db)
	garageRepo := garage.NewRepository(db)
	contactsRepo := contacts.NewRepository(db)
	eventsRepo := events.NewRepository(db)
	pointsRepo := points.NewRepository(db)
	reviewsRepo := reviews.NewRepository(db)
	routesRepo := routes.NewRepository(db)
	pushRepo := push.NewRepository(db)

	// Shared SMTP transport for login codes + notification emails.
	mail := mailer.New(cfg)
	// Web Push (VAPID); falls back to email when keys are unset.
	pushSvc := push.NewService(pushRepo, cfg.VAPIDPublic, cfg.VAPIDPrivate, cfg.VAPIDSubject)

	// Services
	authSvc := auth.NewService(authRepo, cfg, mail)
	profileSvc := profile.NewService(profileRepo)
	geoSvc := geo.NewService(geoRepo)
	chatSvc := chat.NewService(chatRepo, profileSvc, mail, pushSvc, cfg.UploadBaseURL)
	garageSvc := garage.NewService(garageRepo)
	contactsSvc := contacts.NewService(contactsRepo, profileSvc)
	eventsSvc := events.NewService(eventsRepo, profileSvc, mail, pushSvc, cfg.UploadBaseURL)
	pointsSvc := points.NewService(pointsRepo)
	reviewsSvc := reviews.NewService(reviewsRepo)
	routesSvc := routes.NewService(routesRepo)

	hub := chat.NewHub()
	go hub.Run()

	// Handlers
	authHandler := auth.NewHandler(authSvc)
	profileHandler := profile.NewHandler(profileSvc)
	geoHandler := geo.NewHandler(geoSvc)
	chatHandler := chat.NewHandler(chatSvc, hub)
	garageHandler := garage.NewHandler(garageSvc)
	contactsHandler := contacts.NewHandler(contactsSvc)
	eventsHandler := events.NewHandler(eventsSvc)
	pointsHandler := points.NewHandler(pointsSvc)
	reviewsHandler := reviews.NewHandler(reviewsSvc)
	routesHandler := routes.NewHandler(routesSvc)
	pushHandler := push.NewHandler(pushSvc)

	mediaHandler, err := media.NewHandler(cfg.UploadDir, cfg.UploadBaseURL)
	if err != nil {
		log.Fatalf("media handler: %v", err)
	}

	wsHandler := chat.NewWSHandler(hub, chatSvc, geoSvc, profileSvc)

	authMW := middleware.NewAuth(cfg.JWTSecret)

	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.CORS(cfg.CORSOrigins))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Static file serving for uploads
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadDir))))

	// Public
	r.Route("/auth", func(r chi.Router) {
		r.Post("/send-code", authHandler.SendCode)
		r.Post("/verify", authHandler.Verify)
	})

	// Public map reads: points of interest and riders are viewable by anyone,
	// even unauthenticated. Mutations (writing your own location, creating/editing
	// points) stay behind auth via a nested group.
	r.Route("/geo", func(r chi.Router) {
		r.Get("/users", geoHandler.GetUsersOnMap)
		r.Group(func(r chi.Router) {
			r.Use(authMW.RequireAuth)
			r.Put("/location", geoHandler.UpdateLocation)
		})
	})

	// "Motorcyclists" is not a stored point — it's the /geo/users layer above,
	// toggled client-side.
	r.Route("/points", func(r chi.Router) {
		r.Get("/", pointsHandler.List)
		r.Get("/{id}", pointsHandler.Get)
		r.Get("/{id}/reviews", reviewsHandler.List)
		r.Group(func(r chi.Router) {
			r.Use(authMW.RequireAuth)
			r.Post("/", pointsHandler.Create)
			r.Patch("/{id}", pointsHandler.Update)
			r.Delete("/{id}", pointsHandler.Delete)
			r.Post("/{id}/reviews", reviewsHandler.Upsert)
			r.Delete("/{id}/reviews", reviewsHandler.Delete)
		})
	})

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(authMW.RequireAuth)

		r.Route("/profile", func(r chi.Router) {
			r.Get("/me", profileHandler.GetMe)
			r.Patch("/me", profileHandler.UpdateMe)
			r.Get("/search", profileHandler.Search)
			r.Get("/by-username/{username}", profileHandler.GetByUsername)
			r.Get("/{userID}", profileHandler.GetUser)
		})

		r.Route("/chat", func(r chi.Router) {
			r.Get("/chats", chatHandler.ListChats)
			r.Post("/chats", chatHandler.CreateChat)
			r.Patch("/chats/{chatID}", chatHandler.UpdateChat)
			r.Post("/chats/{chatID}/members", chatHandler.AddMember)
			r.Delete("/chats/{chatID}/members/{userID}", chatHandler.RemoveMember)
			r.Get("/chats/{chatID}/messages", chatHandler.GetMessages)
			r.Post("/chats/{chatID}/messages", chatHandler.SendMessage)
			r.Post("/chats/{chatID}/read", chatHandler.MarkRead)
			r.Patch("/chats/{chatID}/messages/{messageID}", chatHandler.EditMessage)
			r.Delete("/chats/{chatID}/messages/{messageID}", chatHandler.DeleteMessage)
		})

		r.Route("/contacts", func(r chi.Router) {
			r.Get("/", contactsHandler.List)
			r.Post("/", contactsHandler.Add)
			r.Delete("/{id}", contactsHandler.Delete)
		})

		r.Route("/garage", func(r chi.Router) {
			r.Get("/", garageHandler.List)
			r.Post("/", garageHandler.Add)
			r.Patch("/{id}", garageHandler.UpdateVehicle)
			r.Delete("/{id}", garageHandler.Delete)
			r.Post("/{id}/service", garageHandler.AddServiceItem)
			r.Patch("/{id}/service/{itemID}", garageHandler.UpdateServiceItem)
			r.Post("/{id}/service/{itemID}/reset", garageHandler.ResetServiceItem)
			r.Delete("/{id}/service/{itemID}", garageHandler.DeleteServiceItem)
		})

		r.Route("/events", func(r chi.Router) {
			r.Get("/", eventsHandler.List)
			r.Post("/", eventsHandler.Create)
			r.Get("/{id}", eventsHandler.Get)
			r.Patch("/{id}", eventsHandler.Update)
			r.Delete("/{id}", eventsHandler.Delete)
			r.Post("/{id}/invite", eventsHandler.Invite)
			r.Post("/{id}/respond", eventsHandler.Respond)
			r.Delete("/{id}/participants/{userID}", eventsHandler.RemoveParticipant)
		})

		r.Route("/routes", func(r chi.Router) {
			r.Get("/suggested", routesHandler.ListSuggested) // before /{id} so it is not captured
			r.Get("/", routesHandler.List)
			r.Post("/", routesHandler.Create)
			r.Get("/{id}", routesHandler.Get)
			r.Patch("/{id}", routesHandler.Update)
			r.Delete("/{id}", routesHandler.Delete)
		})

		r.Route("/push", func(r chi.Router) {
			r.Get("/public-key", pushHandler.PublicKey)
			r.Post("/subscription", pushHandler.Register)
			r.Delete("/subscription", pushHandler.Unregister)
		})

		r.Post("/media/upload", mediaHandler.Upload)

		r.Get("/ws", wsHandler.Handle)
	})

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

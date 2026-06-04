package main

import (
	"log"
	"net/http"

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
	"riders-connect/internal/media"
	"riders-connect/internal/middleware"
	"riders-connect/internal/points"
	"riders-connect/internal/profile"
)

func main() {
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

	// Services
	authSvc := auth.NewService(authRepo, cfg)
	profileSvc := profile.NewService(profileRepo)
	geoSvc := geo.NewService(geoRepo)
	chatSvc := chat.NewService(chatRepo, profileSvc)
	garageSvc := garage.NewService(garageRepo)
	contactsSvc := contacts.NewService(contactsRepo, profileSvc)
	eventsSvc := events.NewService(eventsRepo, profileSvc)
	pointsSvc := points.NewService(pointsRepo)

	// Handlers
	authHandler := auth.NewHandler(authSvc)
	profileHandler := profile.NewHandler(profileSvc)
	geoHandler := geo.NewHandler(geoSvc)
	chatHandler := chat.NewHandler(chatSvc)
	garageHandler := garage.NewHandler(garageSvc)
	contactsHandler := contacts.NewHandler(contactsSvc)
	eventsHandler := events.NewHandler(eventsSvc)
	pointsHandler := points.NewHandler(pointsSvc)

	mediaHandler, err := media.NewHandler(cfg.UploadDir, cfg.UploadBaseURL)
	if err != nil {
		log.Fatalf("media handler: %v", err)
	}

	hub := chat.NewHub()
	go hub.Run()
	wsHandler := chat.NewWSHandler(hub, chatSvc, geoSvc, profileSvc)

	authMW := middleware.NewAuth(cfg.JWTSecret)

	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

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

		r.Route("/geo", func(r chi.Router) {
			r.Put("/location", geoHandler.UpdateLocation)
			r.Get("/users", geoHandler.GetUsersOnMap)
		})

		// Points of interest on the map (geo domain). "Motorcyclists" is not a
		// stored point — it's the /geo/users layer, toggled client-side.
		r.Route("/points", func(r chi.Router) {
			r.Get("/", pointsHandler.List)
			r.Post("/", pointsHandler.Create)
			r.Get("/{id}", pointsHandler.Get)
			r.Patch("/{id}", pointsHandler.Update)
			r.Delete("/{id}", pointsHandler.Delete)
		})

		r.Route("/chat", func(r chi.Router) {
			r.Get("/chats", chatHandler.ListChats)
			r.Post("/chats", chatHandler.CreateChat)
			r.Post("/chats/{chatID}/members", chatHandler.AddMember)
			r.Get("/chats/{chatID}/messages", chatHandler.GetMessages)
			r.Post("/chats/{chatID}/messages", chatHandler.SendMessage)
		})

		r.Route("/contacts", func(r chi.Router) {
			r.Get("/", contactsHandler.List)
			r.Post("/", contactsHandler.Add)
			r.Delete("/{id}", contactsHandler.Delete)
		})

		r.Route("/garage", func(r chi.Router) {
			r.Get("/", garageHandler.List)
			r.Post("/", garageHandler.Add)
			r.Delete("/{id}", garageHandler.Delete)
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

		r.Post("/media/upload", mediaHandler.Upload)

		r.Get("/ws", wsHandler.Handle)
	})

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

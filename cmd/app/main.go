package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"AGoTTHT/internal/db"
	"AGoTTHT/internal/handler"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("Note: No .env file found, relying on system environment variables")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/app.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("DB failed to init: %v", err)
	}
	defer database.Close()

	// Session management
	sessionStore := sqlite3store.New(database)
	defer sessionStore.StopCleanup()

	sessionManager := scs.New()
	sessionManager.Lifetime = 24 * time.Hour
	sessionManager.Store = sessionStore
	sessionManager.Cookie.Persist = true
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	if os.Getenv("ENV") == "production" {
		sessionManager.Cookie.Secure = true
	}

	requireAuth := func(h http.HandlerFunc) http.Handler {
		return handler.RequireAuth(sessionManager)(h)
	}
	requireAdmin := func(h http.HandlerFunc) http.Handler {
		return handler.RequireAuth(sessionManager)(
			handler.RequireAdmin(sessionManager)(h),
		)
	}

	// Routes
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fs))
	mux.HandleFunc("GET /api/health", handler.HandleHealth)

	// Auth
	mux.HandleFunc("GET /login", handler.HandleLogin)
	mux.HandleFunc("GET /auth/github", handler.HandleGitHubLogin(sessionManager))
	mux.HandleFunc("GET /auth/github/callback", handler.HandleGitHubCallback(database, sessionManager))
	mux.HandleFunc("POST /logout", handler.HandleLogout(sessionManager))

	// Pages
	mux.Handle("GET /{$}", requireAuth(handler.HandleDesk))
	mux.Handle("GET /dictionary", requireAuth(handler.HandleDictionary))
	mux.Handle("GET /community", requireAuth(handler.HandleCommunity(database)))
	mux.Handle("GET /community/forum", requireAuth(handler.HandleGetForumSection(database)))
	mux.Handle("GET /tutorial", requireAuth(handler.HandleTutorial))
	mux.Handle("GET /admin", requireAdmin(handler.HandleAdmin))
	mux.Handle("/upload", requireAdmin(handler.HandleUpload(database)))
	mux.Handle("GET /help", requireAuth(handler.HandleHelp))
	mux.Handle("GET /settings", requireAuth(handler.HandleSettings(database, sessionManager)))

	// Document operations
	mux.Handle("GET /search", requireAuth(handler.HandleSearch(database)))
	mux.Handle("DELETE /documents/{id}", requireAdmin(handler.HandleDelete(database)))
	mux.Handle("GET /documents/search", requireAuth(handler.HandleSearchDocuments(database)))
	mux.Handle("GET /documents/{id}/load", requireAuth(handler.HandleLoadDocumentPanes(database, sessionManager)))

	// Dictionary API
	mux.Handle("POST /api/dictionary/add", requireAuth(handler.HandleAddUserDictionary(database, sessionManager)))
	mux.Handle("POST /api/dictionary/remove", requireAuth(handler.HandleRemoveUserDictionary(database, sessionManager)))
	mux.Handle("GET /api/dictionary/search", requireAuth(handler.HandleSearchUserDictionary(database, sessionManager)))

	// Community & forum
	mux.Handle("GET /community/new-post", requireAuth(handler.HandleNewPostModal))
	mux.Handle("POST /community/posts", requireAuth(handler.HandleCreatePost(database, sessionManager)))
	mux.Handle("GET /community/threads/{id}", requireAuth(handler.HandleGetThreadDetails(database, sessionManager)))
	mux.Handle("POST /community/threads/{id}/replies", requireAuth(handler.HandleCreateReply(database, sessionManager)))
	mux.Handle("POST /community/threads/{id}/lock", requireAdmin(handler.HandleToggleLockThread(database, sessionManager)))
	mux.Handle("DELETE /community/threads/{id}", requireAdmin(handler.HandleDeleteThread(database, sessionManager)))

	// Settings & account deletion
	mux.Handle("DELETE /settings/dictionary", requireAuth(handler.HandleClearDictionary(database, sessionManager)))
	mux.Handle("DELETE /settings/account", requireAuth(handler.HandleDeleteAccount(database, sessionManager)))

	// Global chat API
	mux.Handle("GET /api/chat/messages", requireAuth(handler.HandleGetChatMessages(database, sessionManager)))
	mux.Handle("POST /api/chat/messages", requireAuth(handler.HandlePostChatMessage(database, sessionManager)))
	mux.Handle("DELETE /api/chat/messages/{id}", requireAdmin(handler.HandleDeleteChatMessage(database, sessionManager)))

	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(`User-agent: *
Allow: /tutorial
Allow: /help

Disallow: /admin
Disallow: /upload
Disallow: /settings
Disallow: /login
Disallow: /auth/
Disallow: /api/
Disallow: /static/
`))
	})

	appHandler := sessionManager.LoadAndSave(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           appHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Server running on http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Server error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Forced server shutdown: %v", err)
	}
}

package main

import (
	"AGoTTHT/internal/db"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"AGoTTHT/internal/handler"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/joho/godotenv"
)

func main() {
	//dev env
	envErr := godotenv.Load(".env")
	if envErr != nil {
		log.Println("Note: No .env file found, relying on system environment variables")
	}
	//db
	dbPath := "data/app.db"
	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("DB failed to init: %v", err)
	}
	defer database.Close()
	//session management
	sessionManager := scs.New()
	sessionManager.Lifetime = 24 * time.Hour
	sessionManager.Store = sqlite3store.New(database)

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

	//routes
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	mux.HandleFunc("/api/health", handler.HandleHealth)
	//login
	mux.HandleFunc("/login", handler.HandleLogin)
	mux.HandleFunc("/auth/github", handler.HandleGitHubLogin(sessionManager))
	mux.HandleFunc("/auth/github/callback", handler.HandleGitHubCallback(database, sessionManager))
	mux.HandleFunc("/logout", handler.HandleLogout(sessionManager))
	//pages
	mux.Handle("/{$}", requireAuth(handler.HandleDesk))
	mux.Handle("/dictionary", requireAuth(handler.HandleDictionary))
	mux.Handle("/community", requireAuth(handler.HandleCommunity))
	mux.Handle("/tutorial", requireAuth(handler.HandleTutorial))
	mux.Handle("/admin", requireAdmin(handler.HandleAdmin))
	mux.Handle("/upload", requireAdmin(handler.HandleUpload(database)))
	mux.Handle("/help", requireAuth(handler.HandleHelp))
	mux.Handle("/settings", requireAuth(handler.HandleSettings))
	//doc logic
	mux.Handle("GET /search", requireAuth(handler.HandleSearch(database)))
	mux.Handle("DELETE /documents/{id}", requireAdmin(handler.HandleDelete(database)))
	mux.Handle("GET /documents/search", requireAuth(handler.HandleSearchDocuments(database)))
	mux.Handle("GET /documents/{id}/load", requireAuth(handler.HandleLoadDocumentPanes(database)))
	sessionHandler := sessionManager.LoadAndSave(mux)
	//server
	fmt.Println("Server running on http://localhost:8080")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running on http://localhost:%s", port)
	err = http.ListenAndServe(":"+port, sessionHandler)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

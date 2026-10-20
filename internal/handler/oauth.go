package handler

import (
	"AGoTTHT/internal/db"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

func getOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}
}

type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

func generateStateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Printf("Failed to generate CSRF state: %v", err)
	}
	return hex.EncodeToString(b)
}

func HandleGitHubLogin(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := generateStateToken()

		sm.Put(r.Context(), "oauth_state", state)

		config := getOAuthConfig()
		url := config.AuthCodeURL(state)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

func HandleGitHubCallback(database *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		storedState := sm.PopString(r.Context(), "oauth_state")
		queryState := r.URL.Query().Get("state")

		if storedState == "" || storedState != queryState {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		config := getOAuthConfig()
		token, err := config.Exchange(r.Context(), code)
		if err != nil {
			log.Printf("OAuth exchange error: %v", err)
			http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
			return
		}

		client := config.Client(r.Context(), token)
		resp, err := client.Get("https://api.github.com/user")
		if err != nil {
			http.Error(w, "Failed to fetch user profile", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		var ghUser GitHubUser
		if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
			http.Error(w, "Failed to parse user profile", http.StatusInternalServerError)
			return
		}

		user, err := db.FindOrCreateUser(database, ghUser.ID, ghUser.Login)
		if err != nil {
			log.Printf("Database error saving user: %v", err)
			http.Error(w, "Failed to store user profile", http.StatusInternalServerError)
			return
		}

		if err := sm.RenewToken(r.Context()); err != nil {
			http.Error(w, "Session error", http.StatusInternalServerError)
			return
		}

		sm.Put(r.Context(), "userID", user.ID)
		sm.Put(r.Context(), "isAdmin", user.IsAdmin)

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func HandleLogout(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = sm.Destroy(r.Context())
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

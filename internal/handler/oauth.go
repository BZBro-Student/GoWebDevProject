package handler

import (
	"AGoTTHT/internal/db"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

const (
	sessionKeyOAuthState = "oauth_state"
	sessionKeyUserID     = "user_id"
	sessionKeyIsAdmin    = "isAdmin"
)

func getOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
		Scopes:       []string{"read:user"},
		Endpoint:     github.Endpoint,
	}
}

type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

func generateStateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate CSRF state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func HandleGitHubLogin(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := generateStateToken()
		if err != nil {
			log.Printf("OAuth state generation error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		sm.Put(r.Context(), sessionKeyOAuthState, state)

		url := getOAuthConfig().AuthCodeURL(state)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

func HandleGitHubCallback(database *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		storedState := sm.PopString(r.Context(), sessionKeyOAuthState)
		queryState := r.URL.Query().Get("state")

		if storedState == "" || subtle.ConstantTimeCompare([]byte(storedState), []byte(queryState)) != 1 {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}

		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			log.Printf("GitHub OAuth denied or failed: %s", oauthErr)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing OAuth code", http.StatusBadRequest)
			return
		}

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
			log.Printf("GitHub profile request error: %v", err)
			http.Error(w, "Failed to fetch user profile", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("GitHub API returned unexpected status: %d", resp.StatusCode)
			http.Error(w, "Failed to fetch user profile", http.StatusBadGateway)
			return
		}

		var ghUser GitHubUser
		if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
			log.Printf("GitHub profile decode error: %v", err)
			http.Error(w, "Failed to parse user profile", http.StatusInternalServerError)
			return
		}

		if ghUser.ID == 0 || ghUser.Login == "" {
			log.Printf("GitHub profile missing required fields: %+v", ghUser)
			http.Error(w, "Invalid user profile received", http.StatusBadGateway)
			return
		}

		user, err := db.FindOrCreateUser(database, ghUser.ID, ghUser.Login)
		if err != nil {
			log.Printf("Database error saving user: %v", err)
			http.Error(w, "Failed to store user profile", http.StatusInternalServerError)
			return
		}

		if err := sm.RenewToken(r.Context()); err != nil {
			log.Printf("Session renewal error: %v", err)
			http.Error(w, "Session error", http.StatusInternalServerError)
			return
		}

		sm.Put(r.Context(), sessionKeyUserID, fmt.Sprint(user.ID))
		sm.Put(r.Context(), sessionKeyIsAdmin, user.IsAdmin)

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func HandleLogout(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := sm.Destroy(r.Context()); err != nil {
			log.Printf("Session destroy error: %v", err)
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

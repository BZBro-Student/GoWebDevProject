package handler

import (
	"log"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

func RequireAuth(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !sm.Exists(r.Context(), "userID") {
				// If the request comes from htmx, instruct the browser to do a full page redirect
				if r.Header.Get("HX-Request") == "true" {
					w.Header().Set("HX-Redirect", "/login")
					w.WriteHeader(http.StatusOK)
					return
				}

				// Standard full-page browser navigation
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			// Prevent caching of authenticated pages
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAdmin(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := sm.GetString(r.Context(), "userID")
			isAdmin := sm.GetBool(r.Context(), "isAdmin")

			log.Printf("[DEBUG Admin Check] UserID: '%s' | IsAdmin: %v", userID, isAdmin)

			if !isAdmin {
				http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

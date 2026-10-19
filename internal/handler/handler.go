package handler

import (
	"AGoTTHT/view/components"
	"AGoTTHT/view/layout"
	"net/http"

	"github.com/a-h/templ"
)

// renderHelper decides whether to send a partial or full page
func renderHelper(w http.ResponseWriter, r *http.Request, component templ.Component) {
	if r.Header.Get("HX-Request") == "true" {
		component.Render(r.Context(), w)
	} else {
		layout.Base(component).Render(r.Context(), w)
	}
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/login" {
		http.NotFound(w, r)
		return
	}

	// Render your login component or page
	renderHelper(w, r, components.Login())
}

// Page handlers
func HandleDesk(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Desk())
}

func HandleDictionary(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dictionary" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Dictionary())
}

func HandleCommunity(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/community" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Community())
}

func HandleTutorial(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/tutorial" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Tutorial())
}

func HandleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Admin())
}

func HandleHelp(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/help" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Help())
}

func HandleSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Settings())
}

// Health API
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/health" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

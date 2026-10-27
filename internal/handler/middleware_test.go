package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"AGoTTHT/internal/handler"

	"github.com/alexedwards/scs/v2"
)

// Helper to initialize a test SessionManager
func setupSessionManager() *scs.SessionManager {
	sm := scs.New()
	sm.Lifetime = 24 * time.Hour
	return sm
}

// Helper to issue a valid session cookie with pre-populated values
func getSessionCookie(t *testing.T, sm *scs.SessionManager, values map[string]any) *http.Cookie {
	t.Helper()

	setupHandler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range values {
			switch val := v.(type) {
			case string:
				sm.Put(r.Context(), k, val)
			case bool:
				sm.Put(r.Context(), k, val)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	setupHandler.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("Failed to generate session cookie")
	}
	return cookies[0]
}

// ==========================================
// RequireAuth Tests
// ==========================================

func TestRequireAuth_Unauthenticated_Standard(t *testing.T) {
	sm := setupSessionManager()

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	testHandler := sm.LoadAndSave(handler.RequireAuth(sm)(nextHandler))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()

	testHandler.ServeHTTP(rec, req)

	if nextCalled {
		t.Error("Expected next handler NOT to be called")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303 See Other, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Expected Location header '/login', got '%s'", loc)
	}
}

func TestRequireAuth_Unauthenticated_HTMX(t *testing.T) {
	sm := setupSessionManager()

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	testHandler := sm.LoadAndSave(handler.RequireAuth(sm)(nextHandler))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("HX-Request", "true") // Simulate HTMX request
	rec := httptest.NewRecorder()

	testHandler.ServeHTTP(rec, req)

	if nextCalled {
		t.Error("Expected next handler NOT to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", rec.Code)
	}
	if hxRedirect := rec.Header().Get("HX-Redirect"); hxRedirect != "/login" {
		t.Errorf("Expected HX-Redirect header '/login', got '%s'", hxRedirect)
	}
}

func TestRequireAuth_Authenticated(t *testing.T) {
	sm := setupSessionManager()
	cookie := getSessionCookie(t, sm, map[string]any{
		"userID": "user-123",
	})

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	testHandler := sm.LoadAndSave(handler.RequireAuth(sm)(nextHandler))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	testHandler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("Expected next handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", rec.Code)
	}
	expectedCacheHeader := "no-store, no-cache, must-revalidate"
	if cacheHeader := rec.Header().Get("Cache-Control"); cacheHeader != expectedCacheHeader {
		t.Errorf("Expected Cache-Control '%s', got '%s'", expectedCacheHeader, cacheHeader)
	}
}

// ==========================================
// RequireAdmin Tests
// ==========================================

func TestRequireAdmin_NonAdmin(t *testing.T) {
	sm := setupSessionManager()
	cookie := getSessionCookie(t, sm, map[string]any{
		"userID":  "user-123",
		"isAdmin": false,
	})

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	testHandler := sm.LoadAndSave(handler.RequireAdmin(sm)(nextHandler))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	testHandler.ServeHTTP(rec, req)

	if nextCalled {
		t.Error("Expected next handler NOT to be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden, got %d", rec.Code)
	}
}

func TestRequireAdmin_IsAdmin(t *testing.T) {
	sm := setupSessionManager()
	cookie := getSessionCookie(t, sm, map[string]any{
		"userID":  "admin-123",
		"isAdmin": true,
	})

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	testHandler := sm.LoadAndSave(handler.RequireAdmin(sm)(nextHandler))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	testHandler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("Expected next handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", rec.Code)
	}
}

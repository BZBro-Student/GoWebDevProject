package handler_test

import (
	"AGoTTHT/internal/handler"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleLoginPage(t *testing.T) {

	req := httptest.NewRequest(http.MethodGet, "/login", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}
func TestHandleDeskPage(t *testing.T) {

	req := httptest.NewRequest(http.MethodGet, "/desk", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleDictionaryPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dictionary", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleCommunityPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/community", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleTutorialPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tutorial", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleAdminPage(t *testing.T) {

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleHelpPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/help", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleSettingsPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)

	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

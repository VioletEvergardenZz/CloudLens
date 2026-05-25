package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

func TestWithCORS_EmptyConfig_AllowsAnyOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: ""}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Header.Set("Origin", "http://any-origin.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://any-origin.example.com" {
		t.Fatalf("expected allow origin header set, got %q", got)
	}
}

func TestWithCORS_ExplicitAllowList_StillEnforced(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: "http://localhost:5173"}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Header.Set("Origin", "http://localhost:5174")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestWithCORS_OptionsWhenOriginDenied_ReturnsForbidden(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: "http://localhost:5173"}, next)

	req := httptest.NewRequest(http.MethodOptions, "/api/config", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestWithCORS_AllowHeaders_NoAPITokenHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: ""}, next)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	headers := rec.Header().Get("Access-Control-Allow-Headers")
	if headers != "Content-Type,Authorization" {
		t.Fatalf("unexpected Access-Control-Allow-Headers: %q", headers)
	}
}

func TestWithCORS_AllowMethods_CoversManagementWorkflow(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: ""}, next)

	req := httptest.NewRequest(http.MethodOptions, "/api/cloud/accounts/1", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	methods := rec.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		if !strings.Contains(methods, method) {
			t.Fatalf("Access-Control-Allow-Methods should include %s, got %q", method, methods)
		}
	}
}

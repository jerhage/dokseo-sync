package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func (f *fixture) withOrigin(method, target, origin string, extra map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("Origin", origin)
	for name, value := range extra {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestCORSEchoesAnAllowedOrigin(t *testing.T) {
	f := newFixture(t)
	recorder := f.withOrigin(http.MethodGet, "/v1/info", allowedOrigin, nil)
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("GET /v1/info Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if got := recorder.Header().Get("Vary"); got != "Origin" {
		t.Errorf("GET /v1/info Vary = %q, want %q", got, "Origin")
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("GET /v1/info Allow-Credentials = %q, want none", got)
	}
}

func TestCORSOmitsAllowOriginForOtherOrigins(t *testing.T) {
	f := newFixture(t)
	recorder := f.withOrigin(http.MethodGet, "/v1/info", "https://evil.example", nil)
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("GET /v1/info from another origin Allow-Origin = %q, want none", got)
	}
	if got := recorder.Header().Get("Vary"); got != "Origin" {
		t.Errorf("GET /v1/info from another origin Vary = %q, want %q", got, "Origin")
	}
}

func TestCORSAnswersAnAllowedPreflight(t *testing.T) {
	f := newFixture(t)
	recorder := f.withOrigin(http.MethodOptions, "/v1/changes", allowedOrigin, map[string]string{"Access-Control-Request-Method": "POST"})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /v1/changes status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":  allowedOrigin,
		"Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
		"Access-Control-Allow-Headers": "Authorization, Content-Type",
		"Access-Control-Max-Age":       "86400",
		"Vary":                         "Origin",
	}
	for name, value := range want {
		if got := recorder.Header().Get(name); got != value {
			t.Errorf("OPTIONS /v1/changes %s = %q, want %q", name, got, value)
		}
	}
}

func TestCORSLeavesAPreflightFromAnotherOriginUnanswered(t *testing.T) {
	f := newFixture(t)
	recorder := f.withOrigin(http.MethodOptions, "/v1/changes", "https://evil.example", map[string]string{"Access-Control-Request-Method": "POST"})
	if recorder.Code != http.StatusNotFound {
		t.Errorf("OPTIONS /v1/changes from another origin status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	for _, name := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Max-Age"} {
		if got := recorder.Header().Get(name); got != "" {
			t.Errorf("OPTIONS /v1/changes from another origin %s = %q, want none", name, got)
		}
	}
}

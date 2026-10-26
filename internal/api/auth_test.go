package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChangeEndpointsRejectBadTokens(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name          string
		authorization string
	}{
		{"missing", ""},
		{"wrong scheme", "Token " + testToken},
		{"no token", "Bearer "},
		{"two words", "Bearer " + testToken + " extra"},
		{"unknown", "Bearer nobody"},
		{"revoked", "Bearer " + revokedToken},
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, tc := range tests {
			t.Run(method+" "+tc.name, func(t *testing.T) {
				request := httptest.NewRequest(method, "/v1/changes", nil)
				if tc.authorization != "" {
					request.Header.Set("Authorization", tc.authorization)
				}
				recorder := httptest.NewRecorder()
				f.handler.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusUnauthorized {
					t.Fatalf("%s /v1/changes with %q status = %d, want %d", method, tc.authorization, recorder.Code, http.StatusUnauthorized)
				}
				if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "unauthorized" {
					t.Errorf("%s /v1/changes with %q code = %q, want %q", method, tc.authorization, got, "unauthorized")
				}
				if got := recorder.Header().Get("Content-Type"); got != "application/json" {
					t.Errorf("%s /v1/changes with %q Content-Type = %q, want %q", method, tc.authorization, got, "application/json")
				}
			})
		}
	}
}

func TestAuthenticatedRequestRecordsLastSeen(t *testing.T) {
	f := newFixture(t)
	if recorder := f.do(http.MethodGet, "/v1/changes", testToken, nil); recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/changes status = %d, want %d", recorder.Code, http.StatusOK)
	}
	device, found, err := f.store.DeviceByTokenHash(context.Background(), hashToken(testToken))
	if err != nil || !found {
		t.Fatalf("DeviceByTokenHash() found = %v, error = %v, want found", found, err)
	}
	if device.LastSeenAt == nil || *device.LastSeenAt != testNow {
		t.Errorf("last seen after GET /v1/changes = %v, want %d", device.LastSeenAt, testNow)
	}
}

func TestStoreFailureAnswersInternalWithoutDetails(t *testing.T) {
	f := newFixture(t)
	if err := f.store.Close(); err != nil {
		t.Fatalf("store.Close() error: %v", err)
	}
	recorder := f.do(http.MethodGet, "/v1/changes", testToken, nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("GET /v1/changes with a closed store status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	got := decodeBody[errorResponse](t, recorder).Error
	if got.Code != "internal" || got.Message != "unexpected server failure" {
		t.Errorf("GET /v1/changes with a closed store error = %+v, want code %q and message %q", got, "internal", "unexpected server failure")
	}
	if f.logs.Len() == 0 {
		t.Errorf("GET /v1/changes with a closed store logged nothing, want the failure")
	}
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jerhage/dokseo-sync/internal/store"

	_ "modernc.org/sqlite"
)

const (
	testDeviceID  = "00000000-0000-4000-8000-000000000002"
	otherDevice   = "00000000-0000-4000-8000-000000000003"
	testToken     = "test-token"
	revokedToken  = "revoked-token"
	revokedID     = "00000000-0000-4000-8000-000000000004"
	testNow       = int64(1772445001000)
	testUsername  = "reader"
	testPassword  = "correct horse"
	allowedOrigin = "https://dokseo.example"
)

type fixture struct {
	handler http.Handler
	store   *store.Store
	logs    *bytes.Buffer
	now     time.Time
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Index   *int   `json:"index"`
	} `json:"error"`
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open(%q) error: %v", ":memory:", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("store.Close() error: %v", err)
		}
	})
	ctx := context.Background()
	account, err := s.EnsureAccount(ctx, "reader")
	if err != nil {
		t.Fatalf("EnsureAccount(%q) error: %v", "reader", err)
	}
	devices := []store.NewDevice{
		{ID: testDeviceID, Name: "phone", TokenHash: hashToken(testToken), CreatedAt: 1},
		{ID: revokedID, Name: "old", TokenHash: hashToken(revokedToken), CreatedAt: 2},
	}
	for _, device := range devices {
		if err := s.CreateDevice(ctx, account.ID, device); err != nil {
			t.Fatalf("CreateDevice(%q) error: %v", device.ID, err)
		}
	}
	if _, err := s.RevokeDevice(ctx, account.ID, revokedID, 3); err != nil {
		t.Fatalf("RevokeDevice(%q) error: %v", revokedID, err)
	}
	logs := &bytes.Buffer{}
	f := &fixture{store: s, logs: logs, now: time.UnixMilli(testNow)}
	f.handler = New(s, Options{
		AccountID:      account.ID,
		Now:            func() time.Time { return f.now },
		Credentials:    Credentials{Username: testUsername, Password: testPassword},
		AllowedOrigins: []string{allowedOrigin},
		Logger:         slog.New(slog.NewTextHandler(logs, nil)),
	})
	return f
}

func (f *fixture) do(method, target, token string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func tagChange(id, deviceID string, updatedAt int64) map[string]any {
	return map[string]any{
		"id":        id,
		"kind":      "tag",
		"recordId":  "00000000-0000-4000-8000-000000000005",
		"data":      map[string]any{"name": "grammar", "colour": "sage", "createdAt": 1772445000000},
		"updatedAt": updatedAt,
		"deviceId":  deviceID,
	}
}

func changeID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

func pushBody(t *testing.T, changes ...any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"changes": changes})
	if err != nil {
		t.Fatalf("json.Marshal(push body) error: %v", err)
	}
	return body
}

func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response body %q error: %v", recorder.Body.String(), err)
	}
	return value
}

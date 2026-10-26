package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerhage/dokseo-sync/internal/store"
)

type createdResponse struct {
	DeviceID string `json:"deviceId"`
	Token    string `json:"token"`
}

type listResponse struct {
	Devices []struct {
		DeviceID   string `json:"deviceId"`
		DeviceName string `json:"deviceName"`
		CreatedAt  int64  `json:"createdAt"`
		LastSeenAt *int64 `json:"lastSeenAt"`
	} `json:"devices"`
}

func credentialsBody(t *testing.T, username, password, deviceName string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password, "deviceName": deviceName})
	if err != nil {
		t.Fatalf("json.Marshal(device request) error: %v", err)
	}
	return body
}

func (f *fixture) login(t *testing.T, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	return f.do(http.MethodPost, "/v1/devices", "", credentialsBody(t, username, password, "tablet"))
}

func TestCreateDeviceReturnsATokenThatAuthenticates(t *testing.T) {
	f := newFixture(t)
	recorder := f.login(t, testUsername, testPassword)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("POST /v1/devices status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	created := decodeBody[createdResponse](t, recorder)
	if len(created.DeviceID) != 36 || created.DeviceID[14] != '4' {
		t.Errorf("POST /v1/devices deviceId = %q, want a uuid v4", created.DeviceID)
	}
	if len(created.Token) != 43 {
		t.Errorf("POST /v1/devices token length = %d, want 43", len(created.Token))
	}
	if got := f.do(http.MethodGet, "/v1/changes", created.Token, nil).Code; got != http.StatusOK {
		t.Errorf("GET /v1/changes with the new token status = %d, want %d", got, http.StatusOK)
	}
}

func TestCreateDeviceRejectsWrongCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{"wrong password", testUsername, "wrong"},
		{"wrong username", "someone", testPassword},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			recorder := f.login(t, tc.username, tc.password)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("POST /v1/devices with %s status = %d, want %d", tc.name, recorder.Code, http.StatusUnauthorized)
			}
			if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "unauthorized" {
				t.Errorf("POST /v1/devices with %s code = %q, want %q", tc.name, got, "unauthorized")
			}
		})
	}
}

func TestCreateDeviceRejectsBadBodies(t *testing.T) {
	repeat := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = 'a'
		}
		return string(b)
	}
	tests := []struct {
		name string
		body string
	}{
		{"not json", `{`},
		{"empty body", ``},
		{"missing password", `{"username":"reader","deviceName":"Phone"}`},
		{"missing username", `{"password":"correct horse","deviceName":"Phone"}`},
		{"missing device name", `{"username":"reader","password":"correct horse"}`},
		{"empty device name", `{"username":"reader","password":"correct horse","deviceName":""}`},
		{"number for username", `{"username":1,"password":"correct horse","deviceName":"Phone"}`},
		{"device name too long", `{"username":"reader","password":"correct horse","deviceName":"` + repeat(101) + `"}`},
		{"username too long", `{"username":"` + repeat(129) + `","password":"correct horse","deviceName":"Phone"}`},
		{"password too long", `{"username":"reader","password":"` + repeat(1025) + `","deviceName":"Phone"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			recorder := f.do(http.MethodPost, "/v1/devices", "", []byte(tc.body))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("POST /v1/devices with %s status = %d, want %d", tc.name, recorder.Code, http.StatusBadRequest)
			}
			if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "invalid-request" {
				t.Errorf("POST /v1/devices with %s code = %q, want %q", tc.name, got, "invalid-request")
			}
		})
	}
}

func TestCreateDeviceLimitsFailedAttemptsPerUsername(t *testing.T) {
	f := newFixture(t)
	for attempt := 1; attempt <= 5; attempt++ {
		if got := f.login(t, testUsername, "wrong").Code; got != http.StatusUnauthorized {
			t.Fatalf("POST /v1/devices attempt %d status = %d, want %d", attempt, got, http.StatusUnauthorized)
		}
	}
	recorder := f.login(t, testUsername, "wrong")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("POST /v1/devices attempt 6 status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "rate-limited" {
		t.Errorf("POST /v1/devices attempt 6 code = %q, want %q", got, "rate-limited")
	}
	if got := f.login(t, "other", "wrong").Code; got != http.StatusUnauthorized {
		t.Errorf("POST /v1/devices for another username status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestCreateDeviceRefusesTheCorrectPasswordDuringLockout(t *testing.T) {
	f := newFixture(t)
	for range 5 {
		f.login(t, testUsername, "wrong")
	}
	if got := f.login(t, testUsername, testPassword).Code; got != http.StatusTooManyRequests {
		t.Errorf("POST /v1/devices with the correct password during lockout status = %d, want %d", got, http.StatusTooManyRequests)
	}
}

func TestCreateDeviceLockoutEndsAfterFifteenMinutes(t *testing.T) {
	f := newFixture(t)
	for range 5 {
		f.login(t, testUsername, "wrong")
	}
	f.now = f.now.Add(15*time.Minute - time.Millisecond)
	if got := f.login(t, testUsername, testPassword).Code; got != http.StatusTooManyRequests {
		t.Errorf("POST /v1/devices just before 15 minutes status = %d, want %d", got, http.StatusTooManyRequests)
	}
	f.now = f.now.Add(time.Millisecond)
	if got := f.login(t, testUsername, testPassword).Code; got != http.StatusCreated {
		t.Errorf("POST /v1/devices after 15 minutes status = %d, want %d", got, http.StatusCreated)
	}
}

func TestListDevicesReportsLastSeen(t *testing.T) {
	f := newFixture(t)
	created := decodeBody[createdResponse](t, f.login(t, testUsername, testPassword))
	recorder := f.do(http.MethodGet, "/v1/devices", testToken, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/devices status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[listResponse](t, recorder)
	if len(got.Devices) != 2 {
		t.Fatalf("GET /v1/devices returned %d devices, want 2", len(got.Devices))
	}
	if got.Devices[0].DeviceID != testDeviceID || got.Devices[0].DeviceName != "phone" || got.Devices[0].LastSeenAt == nil {
		t.Errorf("GET /v1/devices first device = %+v, want the phone, seen", got.Devices[0])
	}
	if got.Devices[1].DeviceID != created.DeviceID || got.Devices[1].LastSeenAt != nil {
		t.Errorf("GET /v1/devices second device = %+v, want %s with lastSeenAt null", got.Devices[1], created.DeviceID)
	}
	if got.Devices[1].CreatedAt != testNow {
		t.Errorf("GET /v1/devices createdAt = %d, want %d", got.Devices[1].CreatedAt, testNow)
	}
	f.do(http.MethodGet, "/v1/changes", created.Token, nil)
	again := decodeBody[listResponse](t, f.do(http.MethodGet, "/v1/devices", testToken, nil))
	if again.Devices[1].LastSeenAt == nil || *again.Devices[1].LastSeenAt != testNow {
		t.Errorf("GET /v1/devices lastSeenAt after the device's request = %v, want %d", again.Devices[1].LastSeenAt, testNow)
	}
}

func TestListDevicesRequiresAToken(t *testing.T) {
	f := newFixture(t)
	if got := f.do(http.MethodGet, "/v1/devices", "", nil).Code; got != http.StatusUnauthorized {
		t.Errorf("GET /v1/devices without a token status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestRevokeDeviceLetsADeviceRevokeItself(t *testing.T) {
	f := newFixture(t)
	if got := f.do(http.MethodDelete, "/v1/devices/"+testDeviceID, testToken, nil).Code; got != http.StatusNoContent {
		t.Fatalf("DELETE /v1/devices/%s status = %d, want %d", testDeviceID, got, http.StatusNoContent)
	}
	if got := f.do(http.MethodGet, "/v1/changes", testToken, nil).Code; got != http.StatusUnauthorized {
		t.Errorf("GET /v1/changes with the revoked token status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestRevokeDeviceAnswersNotFoundForUnknownIDs(t *testing.T) {
	f := newFixture(t)
	second, err := f.store.EnsureAccount(t.Context(), "second")
	if err != nil {
		t.Fatalf("EnsureAccount(%q) error: %v", "second", err)
	}
	foreignID := "00000000-0000-4000-8000-000000000009"
	if err := f.store.CreateDevice(t.Context(), second.ID, store.NewDevice{ID: foreignID, Name: "x", TokenHash: "h", CreatedAt: 1}); err != nil {
		t.Fatalf("CreateDevice(%q) error: %v", foreignID, err)
	}
	tests := []struct {
		name string
		id   string
	}{
		{"unknown id", otherDevice},
		{"another account's id", foreignID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := f.do(http.MethodDelete, "/v1/devices/"+tc.id, testToken, nil)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("DELETE /v1/devices/%s status = %d, want %d", tc.id, recorder.Code, http.StatusNotFound)
			}
			if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "not-found" {
				t.Errorf("DELETE /v1/devices/%s code = %q, want %q", tc.id, got, "not-found")
			}
		})
	}
}

func TestCreateDeviceAcceptsEveryValidExample(t *testing.T) {
	paths, err := filepath.Glob("../../contract/examples/valid/device-request/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob device-request examples = %v, error = %v, want files", paths, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s error: %v", path, err)
			}
			var request deviceRequest
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatalf("decode %s error: %v", path, err)
			}
			f := newFixture(t)
			f.handler = New(f.store, Options{AccountID: 1, Now: func() time.Time { return f.now }, Credentials: Credentials{Username: request.Username, Password: request.Password}})
			recorder := f.do(http.MethodPost, "/v1/devices", "", body)
			if recorder.Code != http.StatusCreated {
				t.Errorf("POST /v1/devices with %s status = %d, want %d", filepath.Base(path), recorder.Code, http.StatusCreated)
			}
		})
	}
}

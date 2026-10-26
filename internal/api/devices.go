package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/jerhage/dokseo-sync/internal/store"
)

const (
	maxUsernameLength   = 128
	maxPasswordLength   = 1024
	maxDeviceNameLength = 100
	tokenBytes          = 32
)

type deviceRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	DeviceName string `json:"deviceName"`
}

type deviceCreated struct {
	DeviceID string `json:"deviceId"`
	Token    string `json:"token"`
}

type deviceEntry struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	CreatedAt  int64  `json:"createdAt"`
	LastSeenAt *int64 `json:"lastSeenAt"`
}

type deviceList struct {
	Devices []deviceEntry `json:"devices"`
}

func (r deviceRequest) valid() bool {
	return fits(r.Username, maxUsernameLength) && fits(r.Password, maxPasswordLength) && fits(r.DeviceName, maxDeviceNameLength)
}

func fits(value string, max int) bool {
	n := utf8.RuneCountInString(value)
	return n >= 1 && n <= max
}

func (s *server) checkCredentials(username, password string) bool {
	userSum := sha256.Sum256([]byte(username))
	wantUser := sha256.Sum256([]byte(s.creds.Username))
	passSum := sha256.Sum256([]byte(password))
	wantPass := sha256.Sum256([]byte(s.creds.Password))
	userOK := subtle.ConstantTimeCompare(userSum[:], wantUser[:])
	passOK := subtle.ConstantTimeCompare(passSum[:], wantPass[:])
	return userOK&passOK == 1
}

func (s *server) createDevice(w http.ResponseWriter, r *http.Request) {
	var request deviceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !request.valid() {
		s.writeError(w, http.StatusBadRequest, codeInvalidRequest, invalidRequestMsg)
		return
	}
	now := s.now()
	if s.failures.locked(request.Username, now) {
		s.writeError(w, http.StatusTooManyRequests, codeRateLimited, "too many requests")
		return
	}
	if !s.checkCredentials(request.Username, request.Password) {
		s.failures.record(request.Username, now)
		s.writeError(w, http.StatusUnauthorized, codeUnauthorized, "wrong username or password")
		return
	}
	deviceID, err := newUUID()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	token, err := newToken()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	device := store.NewDevice{ID: deviceID, Name: request.DeviceName, TokenHash: hashToken(token), CreatedAt: now.UnixMilli()}
	if err := s.store.CreateDevice(r.Context(), s.accountID, device); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, deviceCreated{DeviceID: deviceID, Token: token})
}

func (s *server) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.store.ListDevices(r.Context(), s.accountID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	entries := make([]deviceEntry, 0, len(devices))
	for _, d := range devices {
		entries = append(entries, deviceEntry{DeviceID: d.ID, DeviceName: d.Name, CreatedAt: d.CreatedAt, LastSeenAt: d.LastSeenAt})
	}
	s.writeJSON(w, http.StatusOK, deviceList{Devices: entries})
}

func (s *server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	found, err := s.store.RevokeDevice(r.Context(), s.accountID, r.PathValue("deviceId"), s.now().UnixMilli())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, codeNotFound, "no such device")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate device id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func newToken() (string, error) {
	var b [tokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

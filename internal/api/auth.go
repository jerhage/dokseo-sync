package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/jerhage/dokseo-sync/internal/store"
)

type deviceKey struct{}

func deviceFrom(r *http.Request) store.Device {
	device, _ := r.Context().Value(deviceKey{}).(store.Device)
	return device
}

func bearerToken(r *http.Request) (string, bool) {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || token == "" || strings.ContainsAny(token, " \t") {
		return "", false
	}
	return token, true
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			s.writeError(w, http.StatusUnauthorized, codeUnauthorized, "missing, unknown or revoked token")
			return
		}
		device, found, err := s.store.DeviceByTokenHash(r.Context(), hashToken(token))
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !found || device.AccountID != s.accountID {
			s.writeError(w, http.StatusUnauthorized, codeUnauthorized, "missing, unknown or revoked token")
			return
		}
		if err := s.store.TouchDevice(r.Context(), device.AccountID, device.ID, s.now().UnixMilli()); err != nil {
			s.internalError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), deviceKey{}, device)))
	})
}

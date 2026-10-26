package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	codeUnauthorized       = "unauthorized"
	codeDeviceMismatch     = "device-mismatch"
	codeNotFound           = "not-found"
	codeInvalidRequest     = "invalid-request"
	codeInvalidChange      = "invalid-change"
	codeTooLarge           = "too-large"
	codeUnsupportedVersion = "unsupported-version"
	codeRateLimited        = "rate-limited"
	codeInternal           = "internal"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Index   *int   `json:"index,omitempty"`
}

func (s *server) writeJSON(w http.ResponseWriter, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		status = http.StatusInternalServerError
		encoded = []byte(`{"error":{"code":"internal","message":"unexpected server failure"}}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		s.logger.Error("write response", "error", err)
	}
}

func (s *server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func (s *server) writeInvalidChange(w http.ResponseWriter, index int, message string) {
	s.writeJSON(w, http.StatusBadRequest, errorBody{Error: errorDetail{Code: codeInvalidChange, Message: message, Index: &index}})
}

func (s *server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	s.writeError(w, http.StatusInternalServerError, codeInternal, "unexpected server failure")
}

func (s *server) bodyFailure(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		s.writeError(w, http.StatusRequestEntityTooLarge, codeTooLarge, "the request body is over 1 mib")
		return
	}
	s.writeError(w, http.StatusBadRequest, codeInvalidRequest, "the request body could not be read")
}

package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerhage/dokseo-sync/internal/config"
	"github.com/jerhage/dokseo-sync/internal/store"
)

func TestNewServerAnswersInfo(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	}()
	cfg := config.Config{Listen: "127.0.0.1:0", Credentials: config.Credentials{Username: "reader", Password: "hunter2"}}
	srv, err := newServer(context.Background(), cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/info", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("GET /v1/info status = %d, want %d", rec.Code, http.StatusOK)
	}
}

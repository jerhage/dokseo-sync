// Package api serves the dokseo-sync HTTP API described in contract/openapi.yaml.
package api

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/jerhage/dokseo-sync/internal/store"
)

// Version is the server version reported by GET /v1/info. A release sets it with -ldflags -X.
var Version = "0.1.0"

const maxBodyBytes = 1 << 20

var versionedPath = regexp.MustCompile(`^/v([0-9]+)(/|$)`)

// Options configures a server.
type Options struct {
	// AccountID is the account the server serves, as returned by store.EnsureAccount.
	AccountID int64
	// Now returns the current time. It defaults to time.Now.
	Now func() time.Time
	// Logger receives unexpected failures. It defaults to slog.Default.
	Logger *slog.Logger
}

type server struct {
	store     *store.Store
	accountID int64
	now       func() time.Time
	logger    *slog.Logger
}

// New returns the handler that serves the API from s. It returns an http.Handler
// and not an exported type because callers only mount it, which keeps the surface small.
func New(s *store.Store, opts Options) http.Handler {
	srv := &server{store: s, accountID: opts.AccountID, now: opts.Now, logger: opts.Logger}
	if srv.now == nil {
		srv.now = time.Now
	}
	if srv.logger == nil {
		srv.logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/info", srv.info)
	mux.Handle("POST /v1/changes", srv.limitBody(srv.authenticate(http.HandlerFunc(srv.pushChanges))))
	mux.Handle("GET /v1/changes", srv.limitBody(srv.authenticate(http.HandlerFunc(srv.pullChanges))))
	mux.HandleFunc("/", srv.notFound)
	return mux
}

func (s *server) info(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, struct {
		Server      string `json:"server"`
		Version     string `json:"version"`
		APIVersions []int  `json:"apiVersions"`
	}{Server: "dokseo-sync", Version: Version, APIVersions: []int{1}})
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	if m := versionedPath.FindStringSubmatch(r.URL.Path); m != nil && m[1] != "1" {
		s.writeError(w, http.StatusNotFound, codeUnsupportedVersion, "unsupported api version")
		return
	}
	s.writeError(w, http.StatusNotFound, codeNotFound, "no such path")
}

func (s *server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			s.bodyFailure(w, err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

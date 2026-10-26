package api

import (
	"net/http"
	"reflect"
	"testing"
)

func TestUnknownRoutesAnswerNotFoundOrUnsupportedVersion(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		method   string
		path     string
		wantCode string
	}{
		{http.MethodGet, "/v2/changes", "unsupported-version"},
		{http.MethodGet, "/v0/info", "unsupported-version"},
		{http.MethodGet, "/v10/anything/at/all", "unsupported-version"},
		{http.MethodGet, "/v2", "unsupported-version"},
		{http.MethodGet, "/v1/nothing", "not-found"},
		{http.MethodGet, "/nothing", "not-found"},
		{http.MethodGet, "/", "not-found"},
		{http.MethodPost, "/v1/info", "not-found"},
		{http.MethodDelete, "/v1/changes", "not-found"},
		{http.MethodGet, "/vx/changes", "not-found"},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			recorder := f.do(tc.method, tc.path, testToken, nil)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, recorder.Code, http.StatusNotFound)
			}
			if got := decodeBody[errorResponse](t, recorder).Error.Code; got != tc.wantCode {
				t.Errorf("%s %s code = %q, want %q", tc.method, tc.path, got, tc.wantCode)
			}
		})
	}
}

func TestInfoNeedsNoTokenAndNamesTheServer(t *testing.T) {
	f := newFixture(t)
	recorder := f.do(http.MethodGet, "/v1/info", "", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/info status = %d, want %d", recorder.Code, http.StatusOK)
	}
	type info struct {
		Server      string `json:"server"`
		Version     string `json:"version"`
		APIVersions []int  `json:"apiVersions"`
	}
	got := decodeBody[info](t, recorder)
	want := info{Server: "dokseo-sync", Version: Version, APIVersions: []int{1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GET /v1/info body = %+v, want %+v", got, want)
	}
}

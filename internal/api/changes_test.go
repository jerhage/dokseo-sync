package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type acceptedEntry struct {
	ID  string `json:"id"`
	Seq int64  `json:"seq"`
}

type pushResult struct {
	Accepted []acceptedEntry `json:"accepted"`
}

type pullResult struct {
	Changes []map[string]json.RawMessage `json:"changes"`
	Next    int64                        `json:"next"`
	More    bool                         `json:"more"`
}

func (f *fixture) push(t *testing.T, changes ...any) pushResult {
	t.Helper()
	recorder := f.do(http.MethodPost, "/v1/changes", testToken, pushBody(t, changes...))
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /v1/changes status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	return decodeBody[pushResult](t, recorder)
}

func (f *fixture) pull(t *testing.T, query string) pullResult {
	t.Helper()
	recorder := f.do(http.MethodGet, "/v1/changes"+query, testToken, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/changes%s status = %d, want %d, body %s", query, recorder.Code, http.StatusOK, recorder.Body)
	}
	return decodeBody[pullResult](t, recorder)
}

func seqsOf(result pullResult) []string {
	seqs := []string{}
	for _, change := range result.Changes {
		seqs = append(seqs, string(change["seq"]))
	}
	return seqs
}

func TestPushAcceptsChangesInRequestOrder(t *testing.T) {
	f := newFixture(t)
	got := f.push(t,
		tagChange(changeID(2), testDeviceID, 20),
		tagChange(changeID(1), testDeviceID, 10),
	)
	want := pushResult{Accepted: []acceptedEntry{{changeID(2), 1}, {changeID(1), 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("POST /v1/changes accepted = %+v, want %+v", got, want)
	}
}

func TestPushRepeatReturnsTheStoredSeq(t *testing.T) {
	f := newFixture(t)
	first := f.push(t, tagChange(changeID(1), testDeviceID, 10))
	got := f.push(t, tagChange(changeID(1), testDeviceID, 99), tagChange(changeID(2), testDeviceID, 11))
	want := pushResult{Accepted: []acceptedEntry{{changeID(1), first.Accepted[0].Seq}, {changeID(2), 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repeated POST /v1/changes accepted = %+v, want %+v", got, want)
	}
}

func TestPushRejectsADeviceMismatchBeforeAnInvalidChange(t *testing.T) {
	f := newFixture(t)
	invalid := tagChange(changeID(1), testDeviceID, 0)
	foreign := tagChange(changeID(2), otherDevice, 10)
	recorder := f.do(http.MethodPost, "/v1/changes", testToken, pushBody(t, invalid, foreign))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("POST /v1/changes status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	got := decodeBody[errorResponse](t, recorder).Error
	if got.Code != "device-mismatch" || got.Index != nil {
		t.Errorf("POST /v1/changes error = %+v, want code %q and no index", got, "device-mismatch")
	}
}

func TestPushStoresNothingWhenAChangeIsInvalid(t *testing.T) {
	f := newFixture(t)
	body := pushBody(t,
		tagChange(changeID(1), testDeviceID, 10),
		tagChange(changeID(2), testDeviceID, 0),
		tagChange(changeID(3), testDeviceID, 0),
	)
	recorder := f.do(http.MethodPost, "/v1/changes", testToken, body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("POST /v1/changes status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	got := decodeBody[errorResponse](t, recorder).Error
	if got.Code != "invalid-change" || got.Index == nil || *got.Index != 1 {
		t.Errorf("POST /v1/changes error = %+v, want code %q and index 1", got, "invalid-change")
	}
	if pulled := f.pull(t, ""); len(pulled.Changes) != 0 {
		t.Errorf("GET /v1/changes after a rejected push = %d changes, want 0", len(pulled.Changes))
	}
}

func TestPushRejectsChangeCountsOutOfRange(t *testing.T) {
	f := newFixture(t)
	many := make([]any, 501)
	for i := range many {
		many[i] = tagChange(changeID(i+1), testDeviceID, 10)
	}
	tests := []struct {
		name string
		body []byte
	}{
		{"none", pushBody(t)},
		{"501", pushBody(t, many...)},
		{"changes missing", []byte(`{}`)},
		{"not json", []byte(`nope`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := f.do(http.MethodPost, "/v1/changes", testToken, tc.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("POST /v1/changes with %s status = %d, want %d", tc.name, recorder.Code, http.StatusBadRequest)
			}
			if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "invalid-request" {
				t.Errorf("POST /v1/changes with %s code = %q, want %q", tc.name, got, "invalid-request")
			}
		})
	}
}

func TestPushRejectsABodyOverOneMiBBeforeAuth(t *testing.T) {
	f := newFixture(t)
	body := []byte(`{"changes":[],"pad":"` + strings.Repeat("a", 1<<20) + `"}`)
	recorder := f.do(http.MethodPost, "/v1/changes", "", body)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST /v1/changes with %d bytes and no token status = %d, want %d", len(body), recorder.Code, http.StatusRequestEntityTooLarge)
	}
	if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "too-large" {
		t.Errorf("POST /v1/changes with %d bytes code = %q, want %q", len(body), got, "too-large")
	}
}

func TestPushAcceptsEveryContractExample(t *testing.T) {
	f := newFixture(t)
	paths, err := filepath.Glob("../../contract/examples/valid/push-request/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("Glob(valid push-request examples) = %d paths, error %v, want at least one", len(paths), err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) error: %v", path, err)
			}
			var request struct {
				Changes []map[string]any `json:"changes"`
			}
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatalf("decode %q error: %v", path, err)
			}
			changes := make([]any, len(request.Changes))
			for i, change := range request.Changes {
				change["deviceId"] = testDeviceID
				changes[i] = change
			}
			recorder := f.do(http.MethodPost, "/v1/changes", testToken, pushBody(t, changes...))
			if recorder.Code != http.StatusOK {
				t.Errorf("POST /v1/changes with %s status = %d, want %d, body %s", filepath.Base(path), recorder.Code, http.StatusOK, recorder.Body)
			}
		})
	}
}

func TestPullReturnsChangesAfterASeqWithinTheLimit(t *testing.T) {
	f := newFixture(t)
	f.push(t,
		tagChange(changeID(1), testDeviceID, 10),
		tagChange(changeID(2), testDeviceID, 11),
		tagChange(changeID(3), testDeviceID, 12),
	)
	tests := []struct {
		query     string
		wantSeqs  []string
		wantNext  int64
		wantMore  bool
		wantFirst string
	}{
		{"", []string{"1", "2", "3"}, 3, false, changeID(1)},
		{"?after=1", []string{"2", "3"}, 3, false, changeID(2)},
		{"?limit=2", []string{"1", "2"}, 2, true, changeID(1)},
		{"?after=1&limit=1", []string{"2"}, 2, true, changeID(2)},
		{"?after=3", []string{}, 3, false, ""},
		{"?after=7", []string{}, 7, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			got := f.pull(t, tc.query)
			if seqs := seqsOf(got); !reflect.DeepEqual(seqs, tc.wantSeqs) {
				t.Errorf("GET /v1/changes%s seqs = %v, want %v", tc.query, seqs, tc.wantSeqs)
			}
			if got.Next != tc.wantNext || got.More != tc.wantMore {
				t.Errorf("GET /v1/changes%s next, more = %d, %v, want %d, %v", tc.query, got.Next, got.More, tc.wantNext, tc.wantMore)
			}
			if tc.wantFirst != "" && string(got.Changes[0]["id"]) != `"`+tc.wantFirst+`"` {
				t.Errorf("GET /v1/changes%s first id = %s, want %q", tc.query, got.Changes[0]["id"], tc.wantFirst)
			}
		})
	}
}

func TestPullReturnsTheStoredChangeShape(t *testing.T) {
	f := newFixture(t)
	f.push(t, tagChange(changeID(1), testDeviceID, 10))
	got := f.pull(t, "").Changes[0]
	want := map[string]string{
		"id":         `"` + changeID(1) + `"`,
		"kind":       `"tag"`,
		"recordId":   `"00000000-0000-4000-8000-000000000005"`,
		"data":       `{"colour":"sage","createdAt":1772445000000,"name":"grammar"}`,
		"updatedAt":  `10`,
		"deviceId":   `"` + testDeviceID + `"`,
		"seq":        `1`,
		"receivedAt": `1772445001000`,
	}
	gotText := map[string]string{}
	for name, value := range got {
		gotText[name] = string(value)
	}
	if !reflect.DeepEqual(gotText, want) {
		t.Errorf("GET /v1/changes change = %v, want %v", gotText, want)
	}
}

func TestPullReturnsATombstoneWithNullData(t *testing.T) {
	f := newFixture(t)
	tombstone := tagChange(changeID(1), testDeviceID, 10)
	tombstone["data"] = nil
	f.push(t, tombstone)
	got := f.pull(t, "").Changes[0]["data"]
	if string(got) != "null" {
		t.Errorf("GET /v1/changes tombstone data = %s, want null", got)
	}
}

func TestPullRejectsBadQueryValues(t *testing.T) {
	f := newFixture(t)
	queries := []string{"?after=-1", "?after=x", "?after=", "?limit=0", "?limit=1001", "?limit=x", "?after=1.5"}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			recorder := f.do(http.MethodGet, "/v1/changes"+query, testToken, nil)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("GET /v1/changes%s status = %d, want %d", query, recorder.Code, http.StatusBadRequest)
			}
			if got := decodeBody[errorResponse](t, recorder).Error.Code; got != "invalid-request" {
				t.Errorf("GET /v1/changes%s code = %q, want %q", query, got, "invalid-request")
			}
		})
	}
}

package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testNow        = int64(1772445000000)
	examplesDir    = "../../contract/examples"
	defaultKey     = "0123456789abcdef0123456789abcdef"
	defaultUUID    = "00000000-0000-4000-8000-000000000005"
	defaultDevice  = "00000000-0000-4000-8000-000000000002"
	defaultChange  = "00000000-0000-4000-8000-000000000001"
	dataFolderTail = "-data"
)

func readExample(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(examplesDir, path))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = %v, want nil", path, err)
	}
	return raw
}

func exampleFiles(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(examplesDir, folder))
	if err != nil {
		t.Fatalf("os.ReadDir(%q) = %v, want nil", folder, err)
	}
	var paths []string
	for _, entry := range entries {
		paths = append(paths, filepath.Join(folder, entry.Name()))
	}
	if len(paths) == 0 {
		t.Fatalf("os.ReadDir(%q) found no examples, want at least one", folder)
	}
	return paths
}

func exampleData(t *testing.T, path string) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(readExample(t, path), &data); err != nil {
		t.Fatalf("json.Unmarshal(%q) = %v, want nil", path, err)
	}
	return data
}

func envelope(kind, recordID string, data any, updatedAt int64) map[string]any {
	return map[string]any{
		"id":        defaultChange,
		"kind":      kind,
		"recordId":  recordID,
		"data":      data,
		"updatedAt": updatedAt,
		"deviceId":  defaultDevice,
	}
}

func encode(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() = %v, want nil", err)
	}
	return raw
}

func recordIDFor(kind string, data map[string]any) string {
	switch kind {
	case "book":
		if hash, ok := data["contentHash"].(string); ok && syncKeyPattern.MatchString(hash) {
			return hash
		}
		return defaultKey
	case "position":
		return defaultKey
	default:
		return defaultUUID
	}
}

func wrapDataExample(t *testing.T, path string) []byte {
	t.Helper()
	kind := strings.TrimSuffix(filepath.Base(filepath.Dir(path)), dataFolderTail)
	data := exampleData(t, path)
	return encode(t, envelope(kind, recordIDFor(kind, data), data, testNow))
}

func wantFailure(t *testing.T, raw []byte, now int64, want Reason) {
	t.Helper()
	_, failure := Validate(raw, now)
	if failure == nil {
		t.Fatalf("Validate(%s, %d) = valid, want failure %q", raw, now, want)
	}
	if failure.Reason != want {
		t.Errorf("Validate(%s, %d) reason = %q at %q, want %q", raw, now, failure.Reason, failure.Path, want)
	}
}

func wantValid(t *testing.T, raw []byte, now int64) Change {
	t.Helper()
	change, failure := Validate(raw, now)
	if failure != nil {
		t.Fatalf("Validate(%s, %d) = failure %q at %q, want valid", raw, now, failure.Reason, failure.Path)
	}
	return change
}

func TestValidateAgainstContractExamples(t *testing.T) {
	kinds := []string{"book", "capture", "tag", "position", "catalog"}
	type example struct {
		path  string
		raw   []byte
		valid bool
	}
	var examples []example
	for _, path := range exampleFiles(t, "valid/change") {
		examples = append(examples, example{path, readExample(t, path), true})
	}
	for _, folder := range []string{"invalid/change", "rules/change"} {
		for _, path := range exampleFiles(t, folder) {
			examples = append(examples, example{path, readExample(t, path), false})
		}
	}
	for _, kind := range kinds {
		for _, status := range []string{"valid", "invalid"} {
			for _, path := range exampleFiles(t, filepath.Join(status, kind+dataFolderTail)) {
				examples = append(examples, example{path, wrapDataExample(t, path), status == "valid"})
			}
		}
	}
	for _, tc := range examples {
		t.Run(tc.path, func(t *testing.T) {
			_, failure := Validate(tc.raw, testNow)
			if tc.valid && failure != nil {
				t.Errorf("Validate(%s) = failure %q at %q, want valid", tc.path, failure.Reason, failure.Path)
			}
			if !tc.valid && failure == nil {
				t.Errorf("Validate(%s) = valid, want failure", tc.path)
			}
		})
	}
}

func TestValidateReturnsTheChange(t *testing.T) {
	raw := []byte(` { "id": "` + defaultChange + `", "kind": "tag", "recordId": "` + defaultUUID + `",
		"data": { "name": "grammar", "colour": "sage", "createdAt": 5 },
		"updatedAt": 1772445000000, "deviceId": "` + defaultDevice + `", "extra": [1, 2] } `)
	got := wantValid(t, raw, testNow)
	want := Change{
		ID:        defaultChange,
		Kind:      "tag",
		RecordID:  defaultUUID,
		Data:      []byte(`{"name":"grammar","colour":"sage","createdAt":5}`),
		UpdatedAt: testNow,
		DeviceID:  defaultDevice,
	}
	if got.ID != want.ID || got.Kind != want.Kind || got.RecordID != want.RecordID ||
		string(got.Data) != string(want.Data) || got.UpdatedAt != want.UpdatedAt || got.DeviceID != want.DeviceID {
		t.Errorf("Validate(tag change with whitespace and an extra member) = %+v, want %+v", got, want)
	}
}

func TestValidateKeepsNoDataForATombstone(t *testing.T) {
	got := wantValid(t, readExample(t, "valid/change/tombstone.json"), testNow)
	if got.Data != nil {
		t.Errorf("Validate(tombstone).Data = %q, want nil", got.Data)
	}
}

func TestValidateRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Reason
	}{
		{"rejects an empty input", ``, ReasonNotJSON},
		{"rejects text that is not json", `{"id":`, ReasonNotJSON},
		{"rejects a json value that is not an object", `[1]`, ReasonNotJSON},
		{"rejects null in place of the change", `null`, ReasonNotJSON},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantFailure(t, []byte(tc.raw), testNow, tc.want)
		})
	}
}

func TestValidateEnvelopeReasons(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   Reason
	}{
		{"rejects a missing device id", func(c map[string]any) { delete(c, "deviceId") }, ReasonMissingField},
		{"rejects a missing data member", func(c map[string]any) { delete(c, "data") }, ReasonMissingField},
		{"rejects an id that is not a uuid", func(c map[string]any) { c["id"] = "abc" }, ReasonBadPattern},
		{"rejects an uppercase device id", func(c map[string]any) { c["deviceId"] = "ABCDEF01-0000-4000-8000-000000000002" }, ReasonBadPattern},
		{"rejects a kind outside the enum", func(c map[string]any) { c["kind"] = "bookmark" }, ReasonNotAllowedValue},
		{"rejects a kind that is not text", func(c map[string]any) { c["kind"] = 3 }, ReasonWrongType},
		{"rejects an empty record id", func(c map[string]any) { c["recordId"] = "" }, ReasonBadPattern},
		{"rejects a sync key as a tag record id", func(c map[string]any) { c["recordId"] = defaultKey }, ReasonBadPattern},
		{"rejects an updatedAt of zero", func(c map[string]any) { c["updatedAt"] = 0 }, ReasonOutOfRange},
		{"rejects a negative updatedAt", func(c map[string]any) { c["updatedAt"] = -5 }, ReasonOutOfRange},
		{"rejects an updatedAt with a fraction", func(c map[string]any) { c["updatedAt"] = 1.5 }, ReasonWrongType},
		{"rejects an updatedAt that is text", func(c map[string]any) { c["updatedAt"] = "1" }, ReasonWrongType},
		{"rejects data that is text", func(c map[string]any) { c["data"] = "tag" }, ReasonWrongType},
		{"rejects data that is a list", func(c map[string]any) { c["data"] = []any{} }, ReasonWrongType},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			change := exampleData(t, "valid/change/tag.json")
			tc.mutate(change)
			wantFailure(t, encode(t, change), testNow, tc.want)
		})
	}
}

func TestValidateRecordIDPatternPerKind(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		recordID string
	}{
		{"rejects a uuid as a book record id", "book", defaultUUID},
		{"rejects an uppercase sync key as a position record id", "position", strings.ToUpper(defaultKey)},
		{"rejects a sync key as a capture record id", "capture", defaultKey},
		{"rejects a sync key as a catalog record id", "catalog", defaultKey},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := encode(t, envelope(tc.kind, tc.recordID, nil, testNow))
			wantFailure(t, raw, testNow, ReasonBadPattern)
		})
	}
}

func TestValidateUpdatedAtLimit(t *testing.T) {
	data := map[string]any{"name": "grammar", "colour": "sage", "createdAt": 1}
	atLimit := encode(t, envelope("tag", defaultUUID, data, testNow+MaxFutureMillis))
	pastLimit := encode(t, envelope("tag", defaultUUID, data, testNow+MaxFutureMillis+1))
	t.Run("passes an updatedAt exactly 24 hours ahead", func(t *testing.T) {
		wantValid(t, atLimit, testNow)
	})
	t.Run("rejects an updatedAt one millisecond past 24 hours ahead", func(t *testing.T) {
		wantFailure(t, pastLimit, testNow, ReasonUpdatedAtInFuture)
	})
}

func tagWithNameOfLength(length int) map[string]any {
	return map[string]any{"name": strings.Repeat("a", length), "colour": "sage", "createdAt": 1}
}

func TestValidateDataSizeLimit(t *testing.T) {
	base := len(encode(t, tagWithNameOfLength(0)))
	atLimit := encode(t, envelope("tag", defaultUUID, tagWithNameOfLength(MaxDataBytes-base), testNow))
	pastLimit := encode(t, envelope("tag", defaultUUID, tagWithNameOfLength(MaxDataBytes-base+1), testNow))
	t.Run("passes data of exactly 64 KiB", func(t *testing.T) {
		got := wantValid(t, atLimit, testNow)
		if len(got.Data) != MaxDataBytes {
			t.Errorf("len(Validate(64 KiB tag).Data) = %d, want %d", len(got.Data), MaxDataBytes)
		}
	})
	t.Run("rejects data one byte over 64 KiB", func(t *testing.T) {
		wantFailure(t, pastLimit, testNow, ReasonDataTooLarge)
	})
	t.Run("measures data without its whitespace", func(t *testing.T) {
		spaced := strings.Replace(string(atLimit), `"data":`, `"data":   `, 1)
		spaced = strings.Replace(spaced, `"colour":`, `"colour":  `, 1)
		wantValid(t, []byte(spaced), testNow)
	})
}

func TestValidateFailureMessages(t *testing.T) {
	future := encode(t, envelope("tag", defaultUUID, nil, testNow+MaxFutureMillis+1))
	large := encode(t, envelope("tag", defaultUUID, tagWithNameOfLength(MaxDataBytes), testNow))
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{"words an updatedAt too far ahead as the contract example does", future, "updatedAt is more than 24 hours in the future"},
		{"words data over the limit", large, "data is over 64 KiB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, failure := Validate(tc.raw, testNow)
			if failure == nil {
				t.Fatalf("Validate(%s) = valid, want failure", tc.name)
			}
			if got := failure.Message(); got != tc.want {
				t.Errorf("Validate(%s).Message() = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestValidateDataRuleReasons(t *testing.T) {
	setIn := func(path ...any) func(map[string]any, any) {
		return func(data map[string]any, value any) {
			var node any = data
			for _, step := range path[:len(path)-1] {
				switch key := step.(type) {
				case string:
					node = node.(map[string]any)[key]
				case int:
					node = node.([]any)[key]
				}
			}
			node.(map[string]any)[path[len(path)-1].(string)] = value
		}
	}
	remove := func(path ...any) func(map[string]any, any) {
		return func(data map[string]any, _ any) {
			var node any = data
			for _, step := range path[:len(path)-1] {
				switch key := step.(type) {
				case string:
					node = node.(map[string]any)[key]
				case int:
					node = node.([]any)[key]
				}
			}
			delete(node.(map[string]any), path[len(path)-1].(string))
		}
	}
	tests := []struct {
		name   string
		kind   string
		file   string
		mutate func(map[string]any, any)
		value  any
		want   Reason
	}{
		{"rejects a book without a title", "book", "valid/book-data/plain.json", remove("title"), nil, ReasonMissingField},
		{"rejects a book with a numeric title", "book", "valid/book-data/plain.json", setIn("title"), 4, ReasonWrongType},
		{"rejects a book with an unknown language", "book", "valid/book-data/plain.json", setIn("language"), "fr", ReasonNotAllowedValue},
		{"rejects a book with a negative image count", "book", "valid/book-data/plain.json", setIn("imageCount"), -1, ReasonOutOfRange},
		{"rejects a book with a fractional image count", "book", "valid/book-data/plain.json", setIn("imageCount"), 1.5, ReasonWrongType},
		{"rejects a book with an empty series id", "book", "valid/book-data/plain.json", setIn("seriesId"), "", ReasonOutOfRange},
		{"rejects a book origin without an acquisition", "book", "valid/book-data/with-origin.json", remove("origin", "acquisition"), nil, ReasonMissingField},
		{"rejects a book origin with a null value", "book", "valid/book-data/with-origin.json", setIn("origin"), nil, ReasonWrongType},
		{"rejects a book origin whose catalog id is not a uuid", "book", "valid/book-data/with-origin.json", setIn("origin", "catalogId"), "x", ReasonBadPattern},
		{"rejects an acquisition format outside the enum", "book", "valid/book-data/with-origin.json", setIn("origin", "acquisition", "format"), "mobi", ReasonNotAllowedValue},
		{"rejects a feed step without an href", "book", "valid/book-data/with-origin.json", remove("origin", "feedPath", 0, "href"), nil, ReasonMissingField},
		{"rejects a capture whose rect has no width", "capture", "valid/capture-data/recognized.json", setIn("anchor", "regions", 0, "rect", "width"), 0, ReasonOutOfRange},
		{"rejects a capture whose rect starts left of the page", "capture", "valid/capture-data/recognized.json", setIn("anchor", "regions", 0, "rect", "x"), -0.1, ReasonOutOfRange},
		{"rejects a capture with no regions", "capture", "valid/capture-data/recognized.json", setIn("anchor", "regions"), []any{}, ReasonTooFewItems},
		{"rejects a capture with an unknown anchor kind", "capture", "valid/capture-data/recognized.json", setIn("anchor", "kind"), "line", ReasonUnknownVariant},
		{"rejects a capture with an unknown origin", "capture", "valid/capture-data/recognized.json", setIn("origin"), "dreamt", ReasonNotAllowedValue},
		{"rejects a capture that repeats a tag id", "capture", "valid/capture-data/recognized.json", setIn("tagIds"), []any{defaultUUID, defaultUUID}, ReasonRepeatedItem},
		{"rejects a capture tag id that is not a uuid", "capture", "valid/capture-data/recognized.json", setIn("tagIds"), []any{"x"}, ReasonBadPattern},
		{"rejects a recognized capture without confidence", "capture", "valid/capture-data/recognized.json", remove("confidence"), nil, ReasonMissingField},
		{"rejects a written capture with a note", "capture", "valid/capture-data/written.json", setIn("note"), nil, ReasonForbiddenField},
		{"rejects a lifted capture with a confidence", "capture", "valid/capture-data/lifted.json", setIn("confidence"), 0.5, ReasonForbiddenField},
		{"rejects a lifted capture without a note", "capture", "valid/capture-data/lifted.json", remove("note"), nil, ReasonMissingField},
		{"rejects a text anchor without a quote", "capture", "valid/capture-data/recognized.json", setIn("anchor"), map[string]any{"kind": "text", "cfi": "c", "chapter": nil}, ReasonMissingField},
		{"rejects a tag with a blank name", "tag", "valid/tag-data/tag.json", setIn("name"), "  ", ReasonBadPattern},
		{"rejects a tag with an unknown colour", "tag", "valid/tag-data/tag.json", setIn("colour"), "teal", ReasonNotAllowedValue},
		{"rejects an image position with an offset above one", "position", "valid/position-data/image.json", setIn("offset"), 1.5, ReasonOutOfRange},
		{"rejects a position with an unknown kind", "position", "valid/position-data/image.json", setIn("kind"), "scroll", ReasonUnknownVariant},
		{"rejects a text position without a cfi", "position", "valid/position-data/text.json", remove("cfi"), nil, ReasonMissingField},
		{"rejects a text position with a fraction above one", "position", "valid/position-data/text.json", setIn("fraction"), 2, ReasonOutOfRange},
		{"rejects a position with a lastReadAt that is text", "position", "valid/position-data/text.json", setIn("lastReadAt"), "now", ReasonWrongType},
		{"rejects a catalog with a password", "catalog", "valid/catalog-data/basic.json", setIn("password"), "secret", ReasonForbiddenField},
		{"rejects a catalog whose auth holds a password", "catalog", "valid/catalog-data/basic.json", setIn("auth", "password"), "secret", ReasonForbiddenField},
		{"rejects a catalog with no auth password but a null password", "catalog", "valid/catalog-data/none.json", setIn("auth", "password"), nil, ReasonForbiddenField},
		{"rejects a basic catalog without a username", "catalog", "valid/catalog-data/basic.json", remove("auth", "username"), nil, ReasonMissingField},
		{"rejects a catalog with an unknown auth kind", "catalog", "valid/catalog-data/basic.json", setIn("auth", "kind"), "token", ReasonUnknownVariant},
		{"rejects a catalog with an unknown protocol", "catalog", "valid/catalog-data/basic.json", setIn("protocol"), "opds2", ReasonNotAllowedValue},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := exampleData(t, tc.file)
			tc.mutate(data, tc.value)
			raw := encode(t, envelope(tc.kind, recordIDFor(tc.kind, data), data, testNow))
			wantFailure(t, raw, testNow, tc.want)
		})
	}
}

func TestValidateBookContentHashMustEqualRecordID(t *testing.T) {
	data := exampleData(t, "valid/book-data/plain.json")
	otherKey := "fedcba9876543210fedcba9876543210"
	t.Run("rejects a book whose content hash differs from its record id", func(t *testing.T) {
		wantFailure(t, encode(t, envelope("book", otherKey, data, testNow)), testNow, ReasonContentHashDiffersFromKey)
	})
	t.Run("passes a tombstone for a book without comparing a hash", func(t *testing.T) {
		wantValid(t, encode(t, envelope("book", otherKey, nil, testNow)), testNow)
	})
}

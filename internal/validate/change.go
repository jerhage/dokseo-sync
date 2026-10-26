package validate

import (
	"bytes"
	"encoding/json"
	"regexp"
)

// Limits the contract sets on one change.
const (
	// MaxDataBytes is the largest data member, measured as compact JSON.
	MaxDataBytes = 64 * 1024
	// MaxFutureMillis is how far ahead of now an updatedAt may be.
	MaxFutureMillis = 24 * 60 * 60 * 1000
)

// Change is a validated change. Its fields match store.NewChange, so the API can convert one to the other. Data is compact JSON, or nil for a tombstone.
type Change struct {
	ID        string
	Kind      string
	RecordID  string
	Data      []byte
	UpdatedAt int64
	DeviceID  string
}

type dataRules func(recordID string, data object) *Failure

func rulesFor(kind string) (*regexp.Regexp, dataRules, bool) {
	switch kind {
	case "book":
		return syncKeyPattern, bookRules, true
	case "position":
		return syncKeyPattern, positionRules, true
	case "capture":
		return uuidPattern, captureRules, true
	case "tag":
		return uuidPattern, tagRules, true
	case "catalog":
		return uuidPattern, catalogRules, true
	default:
		return nil, nil, false
	}
}

// Validate checks one change as sent, in raw JSON, against the contract. now is the server time in Unix milliseconds. It returns the change, or the first rule the change breaks.
func Validate(raw json.RawMessage, now int64) (Change, *Failure) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		return Change{}, fail(ReasonNotJSON, "")
	}
	envelope := make(map[string]any, len(members))
	for name, member := range members {
		var value any
		if err := json.Unmarshal(member, &value); err != nil {
			return Change{}, fail(ReasonNotJSON, name)
		}
		envelope[name] = value
	}
	change := object{fields: envelope}

	id, f := change.text("id")
	if f != nil {
		return Change{}, f
	}
	if f := matching(uuidPattern)(id, "id"); f != nil {
		return Change{}, f
	}
	kind, f := change.text("kind")
	if f != nil {
		return Change{}, f
	}
	recordPattern, rules, known := rulesFor(kind)
	if !known {
		return Change{}, fail(ReasonNotAllowedValue, "kind")
	}
	recordID, f := change.text("recordId")
	if f != nil {
		return Change{}, f
	}
	if f := matching(recordPattern)(recordID, "recordId"); f != nil {
		return Change{}, f
	}
	updatedAt, f := updatedAtRules(change, now)
	if f != nil {
		return Change{}, f
	}
	deviceID, f := change.text("deviceId")
	if f != nil {
		return Change{}, f
	}
	if f := matching(uuidPattern)(deviceID, "deviceId"); f != nil {
		return Change{}, f
	}
	data, f := changeData(change, members["data"], recordID, rules)
	if f != nil {
		return Change{}, f
	}
	return Change{
		ID:        id,
		Kind:      kind,
		RecordID:  recordID,
		Data:      data,
		UpdatedAt: updatedAt,
		DeviceID:  deviceID,
	}, nil
}

func updatedAtRules(change object, now int64) (int64, *Failure) {
	value, f := change.get("updatedAt")
	if f != nil {
		return 0, f
	}
	number, f := integerValue(value, "updatedAt")
	if f != nil {
		return 0, f
	}
	if number <= 0 {
		return 0, fail(ReasonOutOfRange, "updatedAt")
	}
	if number > float64(now+MaxFutureMillis) {
		return 0, fail(ReasonUpdatedAtInFuture, "updatedAt")
	}
	return int64(number), nil
}

func changeData(change object, sent json.RawMessage, recordID string, rules dataRules) ([]byte, *Failure) {
	value, f := change.get("data")
	if f != nil {
		return nil, f
	}
	if value == nil {
		return nil, nil
	}
	data, f := newObject(value, "data")
	if f != nil {
		return nil, f
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, sent); err != nil {
		return nil, fail(ReasonNotJSON, "data")
	}
	if compact.Len() > MaxDataBytes {
		return nil, fail(ReasonDataTooLarge, "data")
	}
	if f := rules(recordID, data); f != nil {
		return nil, f
	}
	return compact.Bytes(), nil
}

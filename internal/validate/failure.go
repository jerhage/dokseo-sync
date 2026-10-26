package validate

import "fmt"

// Reason names the rule a change broke.
type Reason string

// The reasons Validate reports.
const (
	ReasonNotJSON                   Reason = "not-json"
	ReasonMissingField              Reason = "missing-field"
	ReasonWrongType                 Reason = "wrong-type"
	ReasonBadPattern                Reason = "bad-pattern"
	ReasonNotAllowedValue           Reason = "not-allowed-value"
	ReasonOutOfRange                Reason = "out-of-range"
	ReasonTooFewItems               Reason = "too-few-items"
	ReasonRepeatedItem              Reason = "repeated-item"
	ReasonUnknownVariant            Reason = "unknown-variant"
	ReasonForbiddenField            Reason = "forbidden-field"
	ReasonUpdatedAtInFuture         Reason = "updated-at-in-future"
	ReasonDataTooLarge              Reason = "data-too-large"
	ReasonContentHashDiffersFromKey Reason = "content-hash-differs-from-record-id"
)

// Failure is the first broken rule of a change, with the path of the field that broke it.
type Failure struct {
	Reason Reason
	Path   string
}

// Message is the text for the invalid-change error body.
func (f *Failure) Message() string {
	switch f.Reason {
	case ReasonNotJSON:
		return "the change is not valid json"
	case ReasonUpdatedAtInFuture:
		return "updatedAt is more than 24 hours in the future"
	case ReasonDataTooLarge:
		return "data is over 64 KiB"
	case ReasonMissingField:
		return f.Path + " is missing"
	case ReasonWrongType:
		return f.Path + " has the wrong type"
	case ReasonBadPattern:
		return f.Path + " does not match its pattern"
	case ReasonNotAllowedValue:
		return f.Path + " is not an allowed value"
	case ReasonOutOfRange:
		return f.Path + " is out of range"
	case ReasonTooFewItems:
		return f.Path + " has too few items"
	case ReasonRepeatedItem:
		return f.Path + " repeats an item"
	case ReasonUnknownVariant:
		return f.Path + " names an unknown variant"
	case ReasonForbiddenField:
		return f.Path + " is not allowed here"
	case ReasonContentHashDiffersFromKey:
		return f.Path + " differs from recordId"
	default:
		return fmt.Sprintf("%s breaks a rule", f.Path)
	}
}

func fail(reason Reason, path string) *Failure {
	return &Failure{Reason: reason, Path: path}
}

func childPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

func itemPath(parent string, position int) string {
	return fmt.Sprintf("%s[%d]", parent, position)
}

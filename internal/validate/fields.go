package validate

import (
	"math"
	"regexp"
	"slices"
	"strings"
)

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	syncKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type check func(value any, path string) *Failure

type object struct {
	path   string
	fields map[string]any
}

func newObject(value any, path string) (object, *Failure) {
	fields, ok := value.(map[string]any)
	if !ok {
		return object{}, fail(ReasonWrongType, path)
	}
	return object{path: path, fields: fields}, nil
}

func (o object) at(name string) string {
	return childPath(o.path, name)
}

func (o object) has(name string) bool {
	_, present := o.fields[name]
	return present
}

func (o object) get(name string) (any, *Failure) {
	value, present := o.fields[name]
	if !present {
		return nil, fail(ReasonMissingField, o.at(name))
	}
	return value, nil
}

func (o object) require(name string, rule check) *Failure {
	value, f := o.get(name)
	if f != nil {
		return f
	}
	return rule(value, o.at(name))
}

func (o object) optional(name string, rule check) *Failure {
	value, present := o.fields[name]
	if !present {
		return nil
	}
	return rule(value, o.at(name))
}

func (o object) forbid(name string) *Failure {
	if o.has(name) {
		return fail(ReasonForbiddenField, o.at(name))
	}
	return nil
}

func (o object) text(name string) (string, *Failure) {
	value, f := o.get(name)
	if f != nil {
		return "", f
	}
	return textValue(value, o.at(name))
}

func (o object) number(name string) (float64, *Failure) {
	value, f := o.get(name)
	if f != nil {
		return 0, f
	}
	return numberValue(value, o.at(name))
}

func (o object) count(name string) (float64, *Failure) {
	value, f := o.get(name)
	if f != nil {
		return 0, f
	}
	return countValue(value, o.at(name))
}

func (o object) object(name string) (object, *Failure) {
	value, f := o.get(name)
	if f != nil {
		return object{}, f
	}
	return newObject(value, o.at(name))
}

func (o object) array(name string) ([]any, *Failure) {
	value, f := o.get(name)
	if f != nil {
		return nil, f
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fail(ReasonWrongType, o.at(name))
	}
	return items, nil
}

func (o object) nonEmptyArray(name string) ([]any, *Failure) {
	items, f := o.array(name)
	if f != nil {
		return nil, f
	}
	if len(items) == 0 {
		return nil, fail(ReasonTooFewItems, o.at(name))
	}
	return items, nil
}

func textValue(value any, path string) (string, *Failure) {
	text, ok := value.(string)
	if !ok {
		return "", fail(ReasonWrongType, path)
	}
	return text, nil
}

func numberValue(value any, path string) (float64, *Failure) {
	number, ok := value.(float64)
	if !ok {
		return 0, fail(ReasonWrongType, path)
	}
	return number, nil
}

func integerValue(value any, path string) (float64, *Failure) {
	number, f := numberValue(value, path)
	if f != nil {
		return 0, f
	}
	if number != math.Trunc(number) {
		return 0, fail(ReasonWrongType, path)
	}
	return number, nil
}

func countValue(value any, path string) (float64, *Failure) {
	number, f := integerValue(value, path)
	if f != nil {
		return 0, f
	}
	if number < 0 {
		return 0, fail(ReasonOutOfRange, path)
	}
	return number, nil
}

func isText(value any, path string) *Failure {
	_, f := textValue(value, path)
	return f
}

func isNonEmptyText(value any, path string) *Failure {
	text, f := textValue(value, path)
	if f != nil {
		return f
	}
	if text == "" {
		return fail(ReasonOutOfRange, path)
	}
	return nil
}

func isNumber(value any, path string) *Failure {
	_, f := numberValue(value, path)
	return f
}

func isCount(value any, path string) *Failure {
	_, f := countValue(value, path)
	return f
}

func isFraction(value any, path string) *Failure {
	number, f := numberValue(value, path)
	if f != nil {
		return f
	}
	if number < 0 || number > 1 {
		return fail(ReasonOutOfRange, path)
	}
	return nil
}

func isPositiveFraction(value any, path string) *Failure {
	number, f := numberValue(value, path)
	if f != nil {
		return f
	}
	if number <= 0 || number > 1 {
		return fail(ReasonOutOfRange, path)
	}
	return nil
}

func isFilledText(value any, path string) *Failure {
	text, f := textValue(value, path)
	if f != nil {
		return f
	}
	if strings.TrimSpace(text) == "" {
		return fail(ReasonBadPattern, path)
	}
	return nil
}

func matching(pattern *regexp.Regexp) check {
	return func(value any, path string) *Failure {
		text, f := textValue(value, path)
		if f != nil {
			return f
		}
		if !pattern.MatchString(text) {
			return fail(ReasonBadPattern, path)
		}
		return nil
	}
}

func oneOf(allowed ...string) check {
	return func(value any, path string) *Failure {
		text, f := textValue(value, path)
		if f != nil {
			return f
		}
		if !slices.Contains(allowed, text) {
			return fail(ReasonNotAllowedValue, path)
		}
		return nil
	}
}

func orNull(rule check) check {
	return func(value any, path string) *Failure {
		if value == nil {
			return nil
		}
		return rule(value, path)
	}
}

func eachItem(items []any, path string, rule check) *Failure {
	for position, item := range items {
		if f := rule(item, itemPath(path, position)); f != nil {
			return f
		}
	}
	return nil
}

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package jsonvalue renders a decoded JSON value for a human-readable diagnostic.
//
// Two plan-time validators check an author's JSON against a declared schema and have to say, in a
// sentence, what they found where they expected something else: the SDK's appleschemas/profiles
// against Apple's payload key table, and internal/common/aischemas against a vendor's JSON Schema.
// The schema languages share nothing, but the vocabulary for naming a decoded value does — and
// getting it wrong is how a diagnostic ends up reading "expected a integer, found 1e+09".
//
// Everything here operates on the output of encoding/json, so a number arrives as float64 unless the
// decoder was set to use json.Number; both are handled.
package jsonvalue

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Numeric reports whether a value is a JSON number and returns it as a float64.
func Numeric(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

// FormatNumber renders a JSON number without exponent notation, so a large integer reads as the
// digits the author wrote.
//
// The int64 path is taken only inside int64's range, and that bound is this function's own concern
// rather than IsWhole's: converting an out-of-range float to int64 is implementation-defined in Go,
// so 1e30 would otherwise print as int64's minimum instead of its own digits. The comparison is
// strict against MaxInt64 because the constant rounds up to 2^63 as a float64, which does not fit.
func FormatNumber(number float64) string {
	if IsWhole(number) && number >= math.MinInt64 && number < math.MaxInt64 {
		return strconv.FormatInt(int64(number), 10)
	}
	return strconv.FormatFloat(number, 'f', -1, 64)
}

// IsWhole reports whether a JSON number has no fractional part. Integer-ness is a property of the
// value, not of any machine type it might be squeezed into, so 1e30 is as whole as 3 — which is why
// this deliberately carries no int64 bound and every caller that converts to int64 checks that
// bound itself. The round-trip comparison this replaces, `number == float64(int64(number))`, folded
// the two questions together and so called 1e30 fractional on amd64, where an out-of-range
// float-to-int64 conversion is implementation-defined: a schema declaring `"type":"integer"` then
// reported 1e30 as the self-contradictory "expected an integer, found a whole number".
func IsWhole(number float64) bool {
	if math.IsInf(number, 0) || math.IsNaN(number) {
		return false
	}
	return math.Trunc(number) == number
}

// Describe names a value's JSON type for a diagnostic, with its article, so it drops into a sentence
// as "expected a string, found an array".
func Describe(value any) string {
	switch value.(type) {
	case bool:
		return "a boolean"
	case string:
		return "a string"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	case nil:
		return "null"
	default:
		if number, ok := Numeric(value); ok {
			if math.Trunc(number) == number {
				return "a whole number"
			}
			return "a fractional number"
		}
		return "an unexpected value"
	}
}

// Article prefixes a declared type name for readability in a sentence.
//
// The vowel set deliberately omits "u": every name this is called with is a JSON or Apple payload
// type (object, array, integer, number, string, boolean, null, dictionary, data, date, real, any),
// none of which begins with u, and "a unit"-shaped words would take "a" rather than "an" anyway.
func Article(name string) string {
	if name == "" {
		return name
	}
	if strings.ContainsRune("aeio", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}

// Render quotes a value the way an author would recognise it: a string in quotes, a number as its
// digits, anything else by its type. Suits naming the offending value in a diagnostic without
// spilling a whole nested object into it.
func Render(value any) string {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed)
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return "null"
	default:
		if number, ok := Numeric(value); ok {
			return FormatNumber(number)
		}
		return Describe(value)
	}
}

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appledeclarations

import (
	"slices"
	"strings"
	"testing"
)

// TestTableLoads asserts the embedded table decoded and carries provenance. A table that fails to
// decode degrades to an empty one by design, which would silently disable every check below, so
// this is the test that notices.
func TestTableLoads(t *testing.T) {
	table := load()

	if len(table.Declarations) == 0 {
		t.Fatal("embedded declaration table is empty; it failed to decode or was generated wrong")
	}
	if table.Source == "" {
		t.Error("table records no upstream source")
	}
	for _, ref := range table.Refs {
		commit := table.Commits[ref]
		if len(commit) != 40 {
			t.Errorf("ref %q records commit %q, want a 40-character sha", ref, commit)
		}
	}
}

// TestTableRefs asserts the branches the table was built from are the ones SeedOnly's meaning
// depends on: release read first, and anything after it a seed branch.
//
// It deliberately does NOT require a seed branch. Apple promotes its seed branch into release at OS
// GA and deletes it — seed_OS_27_0 went that way, release becoming Release-v27.0 on 2026-09-18 — so
// for part of every year release is the whole vocabulary and a release-only table is the correct
// one. This test used to fail on that, which is a staleness guard it cannot honestly provide
// anyway: whether a seed branch exists is a fact about upstream, and `make test` has no network.
// What it can check is that a ref nobody expected did not arrive, which is what a seed branch alive
// under a name the generator's glob no longer matches would eventually look like.
func TestTableRefs(t *testing.T) {
	refs := Refs()
	if len(refs) == 0 {
		t.Fatal("table records no upstream branch")
	}
	if got := ReleaseRef(); got != "release" {
		t.Errorf("first ref is %q, want %q: the release branch must be read first so SeedOnly means what it says", got, "release")
	}
	for _, ref := range refs[1:] {
		if !strings.HasPrefix(ref, "seed_OS_") {
			t.Errorf("ref %q follows release but is not a seed_OS_* branch; refs are %v", ref, refs)
		}
	}
}

// TestTableCanaries pins keys and types whose provenance proved the seed dependency, so a table
// regenerated from the wrong refs fails here rather than in a user's plan.
func TestTableCanaries(t *testing.T) {
	siri, ok := Lookup("com.apple.configuration.siri.settings")
	if !ok {
		t.Fatal("com.apple.configuration.siri.settings missing from the table")
	}

	// Enabled has been on the release branch for years. AllowSiriAI arrived on seed only, which is
	// what proved the table had to be a union, and reached release with Release-v27.0 — so it is
	// pinned here for its presence rather than its provenance, which moves with Apple.
	for _, name := range []string{"Enabled", "ForceProfanityFilter"} {
		key, ok := siri.Keys[name]
		if !ok {
			t.Fatalf("siri.settings.%s missing from the table", name)
		}
		if key.SeedOnly() {
			t.Errorf("siri.settings.%s reads as seed-only; it is declared on the release branch", name)
		}
	}
	for _, name := range []string{"AllowSiriAI", "ForceReduceSensitiveContent"} {
		key, ok := siri.Keys[name]
		if !ok {
			t.Fatalf("siri.settings.%s missing; the Jamf UI offers it, so a plan rejecting it rejects a working configuration", name)
		}
		if !key.SeedOnly() {
			t.Logf("siri.settings.%s is no longer seed-only; Apple has promoted it to release, which is fine", name)
		}
	}

	// A declaration type that arrived on seed ahead of release, and which a real user configuration
	// relies on.
	if _, ok := Lookup("com.apple.configuration.app.settings"); !ok {
		t.Error("com.apple.configuration.app.settings missing; a type the Jamf UI already offers")
	}
}

// TestTableFloors guards against a generator change that silently drops most of its input. A parser
// that finds nothing reports perfect agreement, so the floors are asserted rather than trusted.
func TestTableFloors(t *testing.T) {
	if got := len(DeclarationTypes()); got < 59 {
		t.Errorf("table holds %d declaration types, want at least 59", got)
	}
	if got := len(StatusItems()); got < 62 {
		t.Errorf("table holds %d status items, want at least 62", got)
	}

	// Every kind Apple defines must be represented, so a whole directory going missing is caught.
	kinds := map[string]bool{}
	for _, name := range DeclarationTypes() {
		declaration, _ := Lookup(name)
		kinds[declaration.Kind] = true
	}
	for _, want := range []string{"CONFIGURATION", "ASSET", "ACTIVATION", "MANAGEMENT"} {
		if !kinds[want] {
			t.Errorf("no %s declarations in the table; a declaration directory is missing", want)
		}
	}
}

// TestConstraintFloors extends the same reasoning as TestTableFloors to the columns that carry the
// checks rather than the entries. Type counts alone cannot catch a generator that stops reading
// Apple's `rangelist`, `range` or `required` annotations: every declaration type and key would
// still be present, every table test above would still pass, and four of the ten findings would
// silently stop firing. A parser that finds nothing reports perfect agreement, so the constraint
// columns are asserted too.
//
// The floors sit well below what the table carries today (112 enums, 17 minima, 27 maxima, 173
// required keys over 937 nodes at the time of writing), so an ordinary upstream refresh moving any
// of them does not trip them; only a collapse does.
func TestConstraintFloors(t *testing.T) {
	var nodes, enums, minima, maxima, required int

	var walk func(schema *Schema)
	walk = func(schema *Schema) {
		if schema == nil {
			return
		}
		nodes++
		if len(schema.Enum) > 0 {
			enums++
		}
		if schema.Min != nil {
			minima++
		}
		if schema.Max != nil {
			maxima++
		}
		if schema.Required {
			required++
		}
		for _, child := range schema.Keys {
			walk(child)
		}
		walk(schema.Any)
		walk(schema.Item)
	}

	for _, name := range DeclarationTypes() {
		declaration, _ := Lookup(name)
		for _, child := range declaration.Keys {
			walk(child)
		}
		walk(declaration.Any)
	}

	for _, floor := range []struct {
		column string
		got    int
		want   int
		check  string
	}{
		{"schema nodes", nodes, 700, "the walk itself"},
		{"enum constraints", enums, 80, "NotInEnum"},
		{"declared minima", minima, 12, "OutOfRange"},
		{"declared maxima", maxima, 20, "OutOfRange"},
		{"required keys", required, 130, "MissingRequiredKey"},
	} {
		if floor.got < floor.want {
			t.Errorf("table carries %d %s, want at least %d; %s would stop firing", floor.got, floor.column, floor.want, floor.check)
		}
	}
}

// TestValidateChecksNamedKeysBesideAWildcard covers a dictionary declaring both a wildcard and
// named keys, which Apple does where a documented core sits alongside vendor extensions. The
// wildcard must suppress only the unknown-key finding: an undeclared name is legal there, while a
// declared one is still type-checked and a required one still required. The declaration table
// carries no such node today, so the schema is built by hand rather than looked up — the shape is
// the point, not the type it belongs to.
func TestValidateChecksNamedKeysBesideAWildcard(t *testing.T) {
	declared := &Schema{
		Type: KindDictionary,
		Any:  &Schema{Type: KindAny},
		Keys: map[string]*Schema{
			"Enabled":    {Type: KindBoolean},
			"MandatedBy": {Type: KindString, Required: true},
		},
	}

	var problems []Problem
	validateDictionary(declared, map[string]any{
		"Enabled":              "yes",
		"vendor.private.token": "anything at all",
	}, "", &problems)

	kinds := map[ProblemKind]string{}
	for _, problem := range problems {
		kinds[problem.Kind] = problem.Path
	}

	if path, ok := kinds[UnknownKey]; ok {
		t.Errorf("unknown key reported at %q beside a wildcard; a free-form name cannot be unknown", path)
	}
	if kinds[WrongType] != "Enabled" {
		t.Errorf("no WrongType reported for Enabled; a named key beside a wildcard must still be type-checked (got %v)", problems)
	}
	if kinds[MissingRequiredKey] != "MandatedBy" {
		t.Errorf("no MissingRequiredKey reported for MandatedBy; a wildcard does not excuse a declared requirement (got %v)", problems)
	}
}

// TestKindForType covers the pairing rule, which needs no table because a declaration type's
// prefix determines its kind.
func TestKindForType(t *testing.T) {
	cases := map[string]string{
		"com.apple.configuration.siri.settings":  "CONFIGURATION",
		"com.apple.asset.credential.certificate": "ASSET",
		"com.apple.activation.simple":            "ACTIVATION",
		"com.apple.management.properties":        "MANAGEMENT",
		"com.example.not.apple":                  "",
	}
	for declarationType, want := range cases {
		if got := KindForType(declarationType); got != want {
			t.Errorf("KindForType(%q) = %q, want %q", declarationType, got, want)
		}
	}
}

// TestValidateAcceptsSpecCompliantDeclarations checks that correct declarations produce nothing. A
// validator that errors on everything is as useless as one that errors on nothing, and these
// payloads were verified against the live service.
func TestValidateAcceptsSpecCompliantDeclarations(t *testing.T) {
	cases := []struct {
		name            string
		kind            string
		declarationType string
		payload         map[string]any
	}{
		{
			name:            "siri settings, release and seed keys together",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.siri.settings",
			payload: map[string]any{
				"Enabled":                     true,
				"ForceProfanityFilter":        true,
				"AllowSiriAI":                 false,
				"ForceReduceSensitiveContent": true,
			},
		},
		{
			name:            "empty payload is legal where no key is required",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         map[string]any{},
		},
		{
			name:            "nested dictionary and enum at the declared value",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.diskmanagement.settings",
			payload: map[string]any{
				"Restrictions": map[string]any{
					"ExternalStorage": "ReadOnly",
					"NetworkStorage":  "Allowed",
				},
			},
		},
		{
			name:            "integers inside their declared range",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.passcode.settings",
			payload: map[string]any{
				"RequirePasscode":       true,
				"MinimumLength":         float64(8),
				"MaximumFailedAttempts": float64(10),
			},
		},
		{
			name:            "asset declaration",
			kind:            "ASSET",
			declarationType: "com.apple.asset.useridentity",
			payload: map[string]any{
				"FullName":     "Dana Whitfield",
				"EmailAddress": "dana.whitfield@example.com",
			},
		},
		{
			name:            "management declaration",
			kind:            "MANAGEMENT",
			declarationType: "com.apple.management.organization-info",
			payload:         map[string]any{"Name": "Example Corp"},
		},
		{
			name:            "status subscription naming published items",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.management.status-subscriptions",
			payload: map[string]any{"StatusItems": []any{
				map[string]any{"Name": "device.identifier.serial-number"},
				map[string]any{"Name": "device.operating-system.version"},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if problems := Validate(tc.kind, tc.declarationType, tc.payload); len(problems) != 0 {
				for _, problem := range problems {
					t.Errorf("unexpected %s at %q: %s", problem.Kind, problem.Path, problem.Detail)
				}
			}
		})
	}
}

// TestValidateRejectsWhatTheServiceDiscards is the behavioural spec, and each case corresponds to a
// step of the constraint probe blueprint whose outcome was read out of the Jamf Pro editor. The
// comments record what the UI showed, because that — not the API's 201 — is the evidence.
func TestValidateRejectsWhatTheServiceDiscards(t *testing.T) {
	cases := []struct {
		name            string
		kind            string
		declarationType string
		payload         map[string]any
		wantKind        ProblemKind
		wantPath        string
	}{
		{
			// Probe 1: keys all lowercase rendered unticked and unset.
			name:            "lowercased key",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         map[string]any{"forceprofanityfilter": true},
			wantKind:        MiscasedKey,
			wantPath:        "forceprofanityfilter",
		},
		{
			// Probe 3: nested keys lowercase, likewise unticked.
			name:            "lowercased nested key",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.diskmanagement.settings",
			payload:         map[string]any{"restrictions": map[string]any{"externalstorage": "ReadOnly"}},
			wantKind:        MiscasedKey,
			wantPath:        "restrictions",
		},
		{
			name:            "invented key",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         map[string]any{"ZzNotAKey": true},
			wantKind:        UnknownKey,
			wantPath:        "ZzNotAKey",
		},
		{
			// Probe 5: the wrong-cased declaration type rendered no card at all.
			name:            "miscased declaration type",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.Siri.Settings",
			payload:         map[string]any{"Enabled": true},
			wantKind:        MiscasedDeclarationType,
		},
		{
			name:            "unknown declaration type",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.not.a.thing",
			payload:         map[string]any{"Enabled": true},
			wantKind:        UnknownDeclarationType,
		},
		{
			// Probe 6: the key ticked, the bogus enum value did not populate.
			name:            "value outside a declared enum",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.diskmanagement.settings",
			payload:         map[string]any{"Restrictions": map[string]any{"ExternalStorage": "Fortnightly"}},
			wantKind:        NotInEnum,
			wantPath:        "Restrictions.ExternalStorage",
		},
		{
			// Probe 8: out-of-range integers rendered as sent. Jamf does not enforce the range; the
			// device does, so this is still an error.
			name:            "integer above the declared maximum",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.passcode.settings",
			payload:         map[string]any{"MinimumLength": float64(999)},
			wantKind:        OutOfRange,
			wantPath:        "MinimumLength",
		},
		{
			// Probe 9: below-minimum integers likewise rendered as sent.
			name:            "integer below the declared minimum",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.passcode.settings",
			payload:         map[string]any{"MaximumFailedAttempts": float64(0)},
			wantKind:        OutOfRange,
			wantPath:        "MaximumFailedAttempts",
		},
		{
			// Probe 10: the key ticked, the wrongly-typed value left the field empty.
			name:            "string where a boolean is declared",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         map[string]any{"Enabled": "yes"},
			wantKind:        WrongType,
			wantPath:        "Enabled",
		},
		{
			name:            "string where an integer is declared",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.passcode.settings",
			payload:         map[string]any{"MinimumLength": "eight"},
			wantKind:        WrongType,
			wantPath:        "MinimumLength",
		},
		{
			name:            "fractional value where an integer is declared",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.passcode.settings",
			payload:         map[string]any{"MinimumLength": 8.5},
			wantKind:        WrongType,
			wantPath:        "MinimumLength",
		},
		{
			name:            "kind disagreeing with the type prefix",
			kind:            "ASSET",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         map[string]any{"Enabled": true},
			wantKind:        KindMismatch,
		},
		{
			name:            "status subscription naming an unpublished item",
			kind:            "CONFIGURATION",
			declarationType: "com.apple.configuration.management.status-subscriptions",
			payload: map[string]any{"StatusItems": []any{
				map[string]any{"Name": "device.identifier.not-a-thing"},
			}},
			wantKind: UnknownStatusItem,
			wantPath: "StatusItems[0].Name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := Validate(tc.kind, tc.declarationType, tc.payload)
			if len(problems) == 0 {
				t.Fatalf("no problems reported; want %s", tc.wantKind)
			}
			found := slices.ContainsFunc(problems, func(p Problem) bool {
				return p.Kind == tc.wantKind && (tc.wantPath == "" || p.Path == tc.wantPath)
			})
			if !found {
				for _, problem := range problems {
					t.Logf("got %s at %q: %s", problem.Kind, problem.Path, problem.Detail)
				}
				t.Errorf("no %s reported at path %q", tc.wantKind, tc.wantPath)
			}
			for _, problem := range problems {
				if problem.Detail == "" {
					t.Errorf("%s at %q carries no detail; the diagnostic would be blank", problem.Kind, problem.Path)
				}
			}
		})
	}
}

// TestValidateReportsMissingRequiredKey covers the one finding that fires on absence rather than on
// something present, so an empty payload is checked rather than skipped.
func TestValidateReportsMissingRequiredKey(t *testing.T) {
	// math.settings declares InputModes.RPN and InputModes.UnitConversion both required, so an
	// InputModes carrying only one is incomplete. This exact omission was a real defect in the
	// hand-written review blueprint, which is why it is pinned here.
	problems := Validate("CONFIGURATION", "com.apple.configuration.math.settings", map[string]any{
		"Calculator": map[string]any{
			"InputModes": map[string]any{"UnitConversion": true},
		},
	})

	found := slices.ContainsFunc(problems, func(p Problem) bool {
		return p.Kind == MissingRequiredKey && p.Path == "Calculator.InputModes.RPN"
	})
	if !found {
		for _, problem := range problems {
			t.Logf("got %s at %q: %s", problem.Kind, problem.Path, problem.Detail)
		}
		t.Error("no MissingRequiredKey reported for Calculator.InputModes.RPN")
	}
}

// TestValidateTreatsNullAsAbsent pins the null handling, which differs from a wrong type: the
// platform discards a null rather than storing it, so a null is never a type error.
func TestValidateTreatsNullAsAbsent(t *testing.T) {
	problems := Validate("CONFIGURATION", "com.apple.configuration.siri.settings", map[string]any{
		"Enabled": nil,
	})
	for _, problem := range problems {
		if problem.Kind == WrongType {
			t.Errorf("null reported as a wrong type at %q: %s", problem.Path, problem.Detail)
		}
	}
}

// TestValidateStopsAtFreeFormDictionaries checks that a dictionary Apple declares as accepting any
// key name is left alone. com.apple.management.properties is the clearest case: its contents are
// the operator's own vocabulary, so nothing there can be unknown.
func TestValidateStopsAtFreeFormDictionaries(t *testing.T) {
	problems := Validate("MANAGEMENT", "com.apple.management.properties", map[string]any{
		"corp.costcentre": "CC-4417",
		"corp.region":     "EMEA",
	})
	for _, problem := range problems {
		if problem.Kind == UnknownKey {
			t.Errorf("free-form key reported as unknown at %q: %s", problem.Path, problem.Detail)
		}
	}
}

// TestMatchesEnumComparesNumbersNumerically guards the trap that JSON decoding turns every number
// into a float64 while Apple declares integer enums as integers: comparing them by representation
// would reject every valid value.
func TestMatchesEnumComparesNumerically(t *testing.T) {
	allowed := []any{float64(14), float64(15), float64(16)}
	for _, value := range []any{float64(14), 14, int64(16)} {
		if !matchesEnum(value, allowed) {
			t.Errorf("matchesEnum(%#v) = false, want true", value)
		}
	}
	if matchesEnum(float64(13), allowed) {
		t.Error("matchesEnum(13) = true, want false")
	}
	if matchesEnum("14", allowed) {
		t.Error(`matchesEnum("14") = true; a string must not satisfy a numeric enum`)
	}
}

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package appledeclarations validates Apple declarative device management declarations at plan time
// against a generated table of Apple's own schemas, built from the `declarative` directory of
// apple/device-management by `make apple-schemas`.
//
// # Why this exists
//
// Jamf validates nothing here. Wire probing on 2026-09-10 established that the blueprints service
// accepts and stores verbatim every malformed declaration put to it — an unknown declaration type,
// an invented key, a wrong-cased key, a value of the wrong type, a value outside a declared enum,
// an integer outside a declared range, a required key omitted — and returns 201 for all of them. A
// deploy of the same blueprint returns 202 and reports SUCCEEDED. The Jamf Pro editor then renders
// the declaration's generated form with the offending key blank, and the device never receives it.
// So the failure is silent at every layer the operator can see, which is exactly why the check
// belongs in `terraform plan`, and why every finding here is an error rather than a warning: there
// is no point letting through a declaration that cannot work on the device.
//
// # How the service behaves, and why the findings are classified as they are
//
// The editor's behaviour distinguishes two stages, which is what made the rules observable: a
// recognised key ticks its own checkbox, and its value populates only if the value is acceptable.
// "Ticked but empty" therefore means the name was understood and the value rejected, while
// "unticked" means the name itself was never recognised.
//
//   - Key names are matched CASE-SENSITIVELY and a wrong-cased key is discarded. This is the
//     opposite of configuration profile payloads, where Jamf silently restores Apple's spelling —
//     see internal/common/appleprofiles. The two packages must not share case handling.
//   - A declaration type is likewise matched case-sensitively; a wrong-cased type renders no card
//     at all.
//   - A value outside a declared `rangelist` is dropped, so an enum violation never applies.
//   - A numeric value outside a declared `range` is NOT rejected: the service stores and renders
//     it unchanged. It is still an error here, because the device is what rejects it, and a warning
//     would leave the operator with a declaration that silently never applies.
//   - `kind` is accepted in any pairing with `type`, so a mismatch reaches the device as a
//     malformed declaration. The pairing is determined by the type's prefix, so no table is needed.
//
// # Snapshot currency
//
// The table is the union of Apple's release branch and its newest seed (pre-release) branch,
// because Jamf's generative-declarations service tracks seed: at the time of writing seed carries
// 12 configuration declaration types and keys such as `siri.settings.AllowSiriAI` that release does
// not, all of them already offered in the Jamf UI. Validating against release alone would reject
// working configurations. Schema.SeedOnly reports which side a key came from so a diagnostic can
// say so.
//
// Because an unrecognised name is an error, a stale table blocks a working configuration. Two
// things mitigate that: the table is refreshed daily by a scheduled pull request, and the provider
// exposes an escape hatch that downgrades these findings, for the window between Apple publishing
// and the provider shipping.
//
// What none of this can prove is that Jamf tracks the *same* seed branch the table unions. That is
// inferred from `AllowSiriAI` appearing in the Jamf picker while absent from release. If false
// positives reappear for keys the UI offers, re-probe that assumption first.
package appledeclarations

import (
	_ "embed"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

//go:embed declarations.json
var embeddedTable []byte

// Kind is the value type Apple declares for a key.
type Kind string

// The declared value types. KindAny accepts anything, and is also what the generator falls back to
// for a type it does not recognise, so an upstream vocabulary change cannot become a false finding.
const (
	KindBoolean    Kind = "boolean"
	KindInteger    Kind = "integer"
	KindReal       Kind = "real"
	KindString     Kind = "string"
	KindData       Kind = "data"
	KindDate       Kind = "date"
	KindArray      Kind = "array"
	KindDictionary Kind = "dictionary"
	KindAny        Kind = "any"
)

// Schema is the declared shape of one key's value.
type Schema struct {
	Type     Kind     `json:"type"`
	Required bool     `json:"required,omitempty"`
	Enum     []any    `json:"enum,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	// Refs names the upstream branches declaring this key, in read order.
	Refs []string           `json:"refs,omitempty"`
	Keys map[string]*Schema `json:"keys,omitempty"`
	Any  *Schema            `json:"any,omitempty"`
	Item *Schema            `json:"item,omitempty"`
}

// SeedOnly reports whether this key is declared only on a pre-release branch.
func (s *Schema) SeedOnly() bool {
	if s == nil || len(s.Refs) == 0 {
		return false
	}
	return !slices.Contains(s.Refs, ReleaseRef())
}

// Declaration is the declared shape of one declaration type.
type Declaration struct {
	Title string             `json:"title,omitempty"`
	Kind  string             `json:"kind"`
	Keys  map[string]*Schema `json:"keys"`
	Any   *Schema            `json:"any,omitempty"`
	Refs  []string           `json:"refs,omitempty"`
}

// SeedOnly reports whether this declaration type is declared only on a pre-release branch.
func (d *Declaration) SeedOnly() bool {
	if d == nil || len(d.Refs) == 0 {
		return false
	}
	return !slices.Contains(d.Refs, ReleaseRef())
}

// Table is the generated declaration table.
type Table struct {
	Source  string            `json:"source"`
	Commits map[string]string `json:"commits"`
	// Refs lists the upstream branches unioned, in read order: the release branch first, the most
	// forward-looking last.
	Refs         []string                `json:"refs"`
	Release      string                  `json:"release,omitempty"`
	Declarations map[string]*Declaration `json:"declarations"`
	// StatusItems is the vocabulary com.apple.configuration.management.status-subscriptions accepts.
	StatusItems []string `json:"statusItems"`
}

// load decodes the embedded table once. A table that fails to decode yields an empty one rather
// than a panic, and an empty table reports nothing: losing validation must never stop a plan.
var load = sync.OnceValue(func() *Table {
	table := &Table{}
	if err := json.Unmarshal(embeddedTable, table); err != nil {
		return &Table{Declarations: map[string]*Declaration{}}
	}
	if table.Declarations == nil {
		table.Declarations = map[string]*Declaration{}
	}
	return table
})

// ReleaseRef returns the branch the table treats as released, which is the first one read.
func ReleaseRef() string {
	refs := load().Refs
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}

// Refs returns the upstream branches the table unions, in read order.
func Refs() []string { return slices.Clone(load().Refs) }

// Provenance returns the commit of the release branch and the upstream release subject, for
// diagnostics that need to say how current the schemas are.
func Provenance() (commit, release string) {
	table := load()
	return table.Commits[ReleaseRef()], table.Release
}

// ProvenanceSummary renders the branches and commits the table was built from, e.g.
// "release@67045e2fa06f + seed_OS_27_0@b0180185a5e4". Preferred over Provenance in a diagnostic:
// Apple's commit subjects are terse internal labels ("Seed8"), which read as a typo when dropped
// into a sentence, whereas a branch and commit are things an operator can actually look up.
func ProvenanceSummary() string {
	table := load()
	parts := make([]string, 0, len(table.Refs))
	for _, ref := range table.Refs {
		commit := table.Commits[ref]
		if len(commit) > 12 {
			commit = commit[:12]
		}
		parts = append(parts, ref+"@"+commit)
	}
	return strings.Join(parts, " + ")
}

// DeclarationTypes returns every declaration type in the table, sorted.
func DeclarationTypes() []string {
	return slices.Sorted(maps.Keys(load().Declarations))
}

// StatusItems returns the subscribable status-item names, sorted.
func StatusItems() []string { return slices.Clone(load().StatusItems) }

// Lookup returns the declared shape of a declaration type. The match is case-sensitive, mirroring
// the service, which renders no form at all for a wrong-cased type.
func Lookup(declarationType string) (*Declaration, bool) {
	declaration, ok := load().Declarations[declarationType]
	return declaration, ok
}

// The kinds a declaration type's reverse-domain prefix implies.
//
// kindConfiguration and kindAsset alias the SDK's generated enum. kindActivation and kindManagement
// are a gateway widening: blueprints.DeclarationKindValues() declares only the first two, while
// POST /blueprints/v1/blueprints on the EU gateway accepted `kind` values ACTIVATION and MANAGEMENT
// on 2026-09-10 under an environment-scoped integration and read both back stored verbatim. So the
// gap is spec-versus-gateway drift rather than a wire break, and the provider must go on deriving
// all four — a declaration type under com.apple.activation. or com.apple.management. has no other
// kind it could carry.
//
// The pattern is internal/providerdata/scopes.go's gatewayWidenings table, and so is the rule for
// maintaining it: an entry is DELETED rather than edited once either half of its justification
// goes. When a spec ingest promotes these two into DeclarationKindValues(), the enum guard in
// enum_literals_test.go fails on the promotion, and the fix is to replace the literal with the new
// constant and delete this paragraph — not to reword it.
const (
	kindConfiguration = blueprints.DeclarationKindConfiguration
	kindAsset         = blueprints.DeclarationKindAsset
	kindActivation    = "ACTIVATION"
	kindManagement    = "MANAGEMENT"
)

// KindForType returns the kind the service expects alongside a declaration type, derived from its
// reverse-domain prefix, or "" when the prefix is not one Apple defines.
func KindForType(declarationType string) string {
	switch {
	case strings.HasPrefix(declarationType, "com.apple.configuration."):
		return kindConfiguration
	case strings.HasPrefix(declarationType, "com.apple.asset."):
		return kindAsset
	case strings.HasPrefix(declarationType, "com.apple.activation."):
		return kindActivation
	case strings.HasPrefix(declarationType, "com.apple.management."):
		return kindManagement
	default:
		return ""
	}
}

// statusSubscriptionsType is the one declaration whose values are checked against the status-item
// vocabulary rather than against a schema.
const statusSubscriptionsType = "com.apple.configuration.management.status-subscriptions"

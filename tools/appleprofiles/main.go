// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Command appleprofiles converts Apple's configuration profile schemas into the compact JSON table
// the provider embeds at internal/common/appleprofiles/profiles.json.
//
// The source is the `mdm/profiles` directory of apple/device-management, which declares, per payload
// type, every key Apple defines and its value type. Jamf's blueprints service validates a legacy
// payload against the same vocabulary, so the table lets the provider catch at plan time what would
// otherwise be a silently discarded key or a failed apply.
//
// Run it through `make apple-profiles`, which pins the upstream checkout and passes the resolved
// commit so the generated table records exactly what it was built from.
//
// Usage:
//
//	go run ./appleprofiles -source <checkout>/mdm/profiles -commit <sha> -release <tag> -out <path>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// wildcardKey is the key name Apple uses for a dictionary that accepts arbitrary key names — an app
// preference domain under com.apple.ManagedClient.preferences, for instance. Everything below one is
// free-form, and the generator stops descending there: Jamf stops validating there too, so a schema
// carried past this point would only manufacture findings the service does not agree with.
const wildcardKey = "ANY"

// maxDepth bounds descent. Two upstream schemas define recursive structures through YAML anchors
// (com.apple.applicationaccess.new, com.apple.homescreenlayout); beyond this depth the subtree is
// recorded as free-form rather than expanded forever.
const maxDepth = 12

// excludedPayloads are the upstream files that do not describe a payload type an author can write.
// CommonPayloadKeys holds the metadata keys every payload carries — those are merged into each
// payload instead (see commonKeys). TopLevel describes the .mobileconfig envelope around payloads,
// not a payload.
var excludedPayloads = map[string]bool{
	"CommonPayloadKeys": true,
	"TopLevel":          true,
}

// specKey is one entry of a payload's `payloadkeys` list, or of a nested `subkeys` list.
type specKey struct {
	Key       string
	Type      string
	Presence  string
	Rangelist []any
	Min       *float64
	Max       *float64
	Subkeys   []*specKey
}

// specFile is one upstream payload schema file.
type specFile struct {
	Title       string
	PayloadType string
	PayloadKeys []*specKey
}

// parseSpec reads one upstream schema file. It walks the YAML at node level rather than unmarshalling
// into structs, because two upstream schemas define a structure that contains itself through a YAML
// anchor (com.apple.applicationaccess.new, com.apple.homescreenlayout) and struct unmarshalling
// rejects those outright. Walking nodes lets the recursion be cut where it closes.
func parseSpec(raw []byte) (*specFile, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	root := document(&doc)
	if root == nil {
		return nil, fmt.Errorf("empty document")
	}

	spec := &specFile{
		Title:       scalar(field(root, "title")),
		PayloadType: scalar(field(document(field(root, "payload")), "payloadtype")),
	}
	spec.PayloadKeys = parseKeys(field(root, "payloadkeys"), map[*yaml.Node]bool{}, 0)
	return spec, nil
}

// parseKeys converts a `payloadkeys` or `subkeys` sequence into spec keys. visiting holds the
// sequence nodes currently on the stack, so a sequence that reaches itself through an alias is cut
// rather than expanded forever; the affected key becomes a free-form dictionary.
//
// depth bounds the walk as well, because visiting only cuts a sequence that reaches *itself*: a
// sequence aliased twice from different points is a directed acyclic graph, and expanding one costs
// 2^n. Upstream's anchors make that reachable — 25 doubling levels of aliases is 3 KB of YAML and
// materialises tens of millions of spec keys. reduce records everything past maxDepth as free-form
// anyway, so cutting the walk at the same bound discards nothing the table would have carried.
func parseKeys(seq *yaml.Node, visiting map[*yaml.Node]bool, depth int) []*specKey {
	seq = document(seq)
	if seq == nil || seq.Kind != yaml.SequenceNode || visiting[seq] || depth >= maxDepth {
		return nil
	}
	visiting[seq] = true
	defer delete(visiting, seq)

	keys := make([]*specKey, 0, len(seq.Content))
	for _, item := range seq.Content {
		item = document(item)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		name := scalar(field(item, "key"))
		minimum, maximum := parseRange(field(item, "range"), name)
		keys = append(keys, &specKey{
			Key:       name,
			Type:      scalar(field(item, "type")),
			Presence:  scalar(field(item, "presence")),
			Rangelist: parseRangelist(field(item, "rangelist"), name),
			Min:       minimum,
			Max:       maximum,
			Subkeys:   parseKeys(field(item, "subkeys"), visiting, depth+1),
		})
	}
	return keys
}

// warnOutput is where warnf writes. It is a variable so a test can capture what a regeneration
// would have printed.
var warnOutput io.Writer = os.Stderr

// warnf records something a regeneration silently worked around — an upstream constraint in a shape
// the generator cannot read, or two branches disagreeing about one. The generator fails soft in
// every such case, since a schema change upstream must not break the build, and a soft failure that
// says nothing is the one that ships a table quietly missing a constraint.
func warnf(format string, args ...any) {
	fmt.Fprintf(warnOutput, "appleprofiles: "+format+"\n", args...)
}

// document follows a document wrapper and resolves an alias to the node it points at, so callers
// always see the mapping, sequence, or scalar itself.
func document(node *yaml.Node) *yaml.Node {
	for node != nil {
		switch {
		case node.Kind == yaml.DocumentNode && len(node.Content) > 0:
			node = node.Content[0]
		case node.Kind == yaml.AliasNode:
			node = node.Alias
		default:
			return node
		}
	}
	return nil
}

// field returns the value node of a mapping key, or nil when the mapping has no such key.
func field(mapping *yaml.Node, name string) *yaml.Node {
	mapping = document(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// scalar returns a node's string value, or "" when the node is absent or not a scalar.
func scalar(node *yaml.Node) string {
	node = document(node)
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

// schema is the emitted shape of one key's value. Exactly one of Keys/Any/Item is populated,
// according to Type.
type schema struct {
	Type     string             `json:"type"`
	Required bool               `json:"required,omitempty"`
	Enum     []any              `json:"enum,omitempty"`
	Min      *float64           `json:"min,omitempty"`
	Max      *float64           `json:"max,omitempty"`
	Keys     map[string]*schema `json:"keys,omitempty"`
	Any      *schema            `json:"any,omitempty"`
	Item     *schema            `json:"item,omitempty"`
	// Refs names the upstream branches that declare this key. A key present only on a
	// pre-release seed branch is still valid — Jamf tracks seed — but the diagnostic says so.
	Refs []string `json:"refs,omitempty"`
}

// payload is the emitted shape of one payload type. Any is set for the handful of payloads whose
// own key list is a wildcard (the ethernet.managed family), meaning any key name is accepted.
type payload struct {
	Title string             `json:"title,omitempty"`
	Keys  map[string]*schema `json:"keys"`
	Any   *schema            `json:"any,omitempty"`
	// Refs names the upstream branches that declare this payload type.
	Refs []string `json:"refs,omitempty"`
}

// table is the emitted table, written to profiles.json.
type table struct {
	Source string `json:"source"`
	// Commits maps each upstream branch read to the commit it was read at, and Refs lists them in
	// read order with the most forward-looking last.
	Commits  map[string]string   `json:"commits"`
	Refs     []string            `json:"refs"`
	Release  string              `json:"release,omitempty"`
	Payloads map[string]*payload `json:"payloads"`
}

// checkout is one upstream branch on disk. Both tables are built from the union of several, because
// Apple publishes new keys on a seed branch well before they reach release and Jamf's services
// track seed — so a table built from release alone reports keys the Jamf UI already offers as
// misspelled, which matters now that an unrecognised key is an error rather than a warning.
type checkout struct {
	ref    string
	commit string
	path   string
}

// rootFlags collects a repeatable `ref=value` flag, preserving the order given so the last branch
// named is the newest.
type rootFlags []string

func (r *rootFlags) String() string { return strings.Join(*r, ",") }

func (r *rootFlags) Set(value string) error {
	if !strings.Contains(value, "=") {
		return fmt.Errorf("expected ref=value, got %q", value)
	}
	*r = append(*r, value)
	return nil
}

// splitRef splits a `ref=value` flag on its first separator, so a value containing one is kept whole.
func splitRef(pair string) (ref, value string) {
	ref, value, _ = strings.Cut(pair, "=")
	return strings.TrimSpace(ref), strings.TrimSpace(value)
}

// resolveCheckouts pairs each -root with its -commit, in the order the roots were given.
func resolveCheckouts(roots, commits rootFlags) ([]checkout, error) {
	shas := make(map[string]string, len(commits))
	for _, pair := range commits {
		ref, sha := splitRef(pair)
		shas[ref] = sha
	}

	resolved := make([]checkout, 0, len(roots))
	for _, pair := range roots {
		ref, path := splitRef(pair)
		if ref == "" || path == "" {
			return nil, fmt.Errorf("-root %q names no ref or no path", pair)
		}
		sha := shas[ref]
		if sha == "" {
			return nil, fmt.Errorf("-root %s has no matching -commit %s=<sha>", ref, ref)
		}
		resolved = append(resolved, checkout{ref: ref, commit: sha, path: path})
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("at least one -root ref=path is required")
	}
	return resolved, nil
}

func main() {
	var roots, commits rootFlags
	flag.Var(&roots, "root", "repeatable `ref=path` naming an apple/device-management checkout root; give release first and the seed branch last")
	flag.Var(&commits, "commit", "repeatable `ref=sha` giving the commit each -root is at")
	release := flag.String("release", "", "upstream release tag or commit subject of the last root")
	profilesOut := flag.String("profiles-out", "", "path to write the configuration profile table to")
	declarationsOut := flag.String("declarations-out", "", "path to write the declaration table to")
	flag.Parse()

	if *profilesOut == "" || *declarationsOut == "" {
		fmt.Fprintln(os.Stderr, "appleprofiles: -profiles-out and -declarations-out are required")
		os.Exit(2)
	}

	checkouts, err := resolveCheckouts(roots, commits)
	if err != nil {
		fmt.Fprintln(os.Stderr, "appleprofiles:", err)
		os.Exit(2)
	}

	profiles, err := generateProfiles(checkouts, *release)
	if err != nil {
		fmt.Fprintln(os.Stderr, "appleprofiles:", err)
		os.Exit(1)
	}
	declarations, err := generateDeclarations(checkouts, *release)
	if err != nil {
		fmt.Fprintln(os.Stderr, "appleprofiles:", err)
		os.Exit(1)
	}

	dropped, err := compareTables(*profilesOut, profiles, *declarationsOut, declarations)
	if err != nil {
		fmt.Fprintln(os.Stderr, "appleprofiles:", err)
		os.Exit(1)
	}
	if len(dropped.Findings) > 0 {
		fmt.Fprintf(os.Stderr, "appleprofiles: this regeneration drops %d name(s) the committed tables carry:\n%s",
			len(dropped.Findings), dropped.report())
	}

	if err := writeTable(*profilesOut, profiles); err != nil {
		fmt.Fprintln(os.Stderr, "appleprofiles:", err)
		os.Exit(1)
	}
	if err := writeTable(*declarationsOut, declarations); err != nil {
		fmt.Fprintln(os.Stderr, "appleprofiles:", err)
		os.Exit(1)
	}

	refs := make([]string, 0, len(checkouts))
	for _, c := range checkouts {
		refs = append(refs, c.ref+"@"+shortSHA(c.commit))
	}
	fmt.Printf("appleprofiles: wrote %d payload types to %s\n", len(profiles.Payloads), *profilesOut)
	fmt.Printf("appleprofiles: wrote %d declaration types and %d status items to %s\n",
		len(declarations.Declarations), len(declarations.StatusItems), *declarationsOut)
	fmt.Printf("appleprofiles: union of %s\n", strings.Join(refs, " + "))
}

// writeTable encodes a table to disk with the indentation the committed artefacts use, so a
// regenerated table diffs line by line against the previous one.
func writeTable(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

// shortSHA abbreviates a commit for the summary line.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// generateProfiles reads every payload schema under each checkout and unions them into the emitted
// table. See checkout for why the union rather than a single branch.
func generateProfiles(roots []checkout, release string) (*table, error) {
	generated := &table{
		Source:   "https://github.com/apple/device-management",
		Commits:  make(map[string]string, len(roots)),
		Release:  release,
		Payloads: make(map[string]*payload),
	}

	for _, root := range roots {
		generated.Refs = append(generated.Refs, root.ref)
		generated.Commits[root.ref] = root.commit
		if err := mergeProfiles(generated, filepath.Join(root.path, "mdm", "profiles"), root.ref); err != nil {
			return nil, err
		}
	}
	if len(generated.Payloads) == 0 {
		return nil, fmt.Errorf("no payload schemas found under %d checkout(s)", len(roots))
	}
	return generated, nil
}

// mergeProfiles folds one checkout's payload schemas into the accumulating table.
func mergeProfiles(generated *table, source, ref string) error {
	entries, err := filepath.Glob(filepath.Join(source, "*.yaml"))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no schema files found under %s", source)
	}
	sort.Strings(entries)

	common, err := commonKeys(source)
	if err != nil {
		return err
	}

	for _, path := range entries {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		spec, err := parseSpec(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}

		payloadType := strings.TrimSpace(spec.PayloadType)
		if payloadType == "" || excludedPayloads[payloadType] {
			continue
		}

		// Several MCX files share one payload type (com.apple.MCX), each describing a different
		// preference domain, so their key sets merge into a single entry.
		entry := generated.Payloads[payloadType]
		if entry == nil {
			entry = &payload{Title: spec.Title, Keys: make(map[string]*schema)}
			generated.Payloads[payloadType] = entry
		}
		entry.Refs = appendRef(entry.Refs, ref)

		for name, keySchema := range dictionaryKeys(spec.PayloadKeys, 1) {
			entry.Keys[name] = unionSchema(entry.Keys[name], keySchema, ref, payloadType+"."+name)
		}
		for _, key := range spec.PayloadKeys {
			if key != nil && key.Key == wildcardKey {
				wildcard := reduce(key, 1)
				wildcard.Required = false
				entry.Any = unionSchema(entry.Any, wildcard, ref, payloadType+"."+wildcardKey)
			}
		}
		for name, keySchema := range common {
			if _, exists := entry.Keys[name]; !exists {
				entry.Keys[name] = unionSchema(nil, keySchema, ref, payloadType+"."+name)
			}
		}
	}

	return nil
}

// commonKeys reads CommonPayloadKeys.yaml, the metadata keys every payload carries. They are merged
// into each payload type because the service accepts them inside a payload and echoes an authored
// value back, so an author who writes one must not see it reported as unknown. Their `required`
// presence is dropped: the provider supplies the identity keys itself, and the service stamps the
// rest, so demanding them from an author's settings would be wrong.
func commonKeys(source string) (map[string]*schema, error) {
	raw, err := os.ReadFile(filepath.Join(source, "CommonPayloadKeys.yaml"))
	if err != nil {
		return nil, err
	}

	spec, err := parseSpec(raw)
	if err != nil {
		return nil, fmt.Errorf("CommonPayloadKeys.yaml: %w", err)
	}

	keys := dictionaryKeys(spec.PayloadKeys, 1)
	for _, keySchema := range keys {
		keySchema.Required = false
	}
	return keys, nil
}

// dictionaryKeys reduces a list of spec keys to the emitted key map, dropping the wildcard entry —
// the caller records that separately, since it applies to any key name rather than one.
func dictionaryKeys(keys []*specKey, depth int) map[string]*schema {
	reduced := make(map[string]*schema, len(keys))
	for _, key := range keys {
		if key == nil || key.Key == "" || key.Key == wildcardKey {
			continue
		}
		reduced[key.Key] = reduce(key, depth)
	}
	return reduced
}

// reduce converts one spec key to its emitted schema, descending into nested dictionaries and array
// element types. An array's element schema is its single subkey: upstream names that subkey for
// documentation (`PayloadContentItem`, `Settings`), but a JSON array is positional, so the name is
// dropped and only the shape is kept.
func reduce(key *specKey, depth int) *schema {
	kind := normaliseType(key.Type)
	result := &schema{
		Type:     kind,
		Required: key.Presence == "required",
		Enum:     key.Rangelist,
		Min:      key.Min,
		Max:      key.Max,
	}

	if depth >= maxDepth {
		result.Type = typeAny
		return result
	}

	switch kind {
	case typeDictionary:
		for _, sub := range key.Subkeys {
			if sub != nil && sub.Key == wildcardKey {
				result.Any = reduce(sub, depth+1)
				result.Any.Required = false
			}
		}
		if named := dictionaryKeys(key.Subkeys, depth+1); len(named) > 0 {
			result.Keys = named
		}
	case typeArray:
		for _, sub := range key.Subkeys {
			if sub == nil {
				continue
			}
			result.Item = reduce(sub, depth+1)
			result.Item.Required = false
			break
		}
	}

	return result
}

// The emitted type vocabulary, mapping Apple's `<...>` spelling to a bare token.
const (
	typeAny        = "any"
	typeArray      = "array"
	typeDictionary = "dictionary"
)

// normaliseType maps Apple's declared type to the emitted token, falling back to the permissive
// `any` for anything unrecognised so a vocabulary change upstream cannot turn into a false finding.
func normaliseType(declared string) string {
	switch strings.ToLower(strings.Trim(declared, "<>")) {
	case "boolean":
		return "boolean"
	case "integer":
		return "integer"
	case "real":
		return "real"
	case "string":
		return "string"
	case "data":
		return "data"
	case "date":
		return "date"
	case "array":
		return typeArray
	case "dictionary":
		return typeDictionary
	default:
		return typeAny
	}
}

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requestLineLimitBytes is the ceiling the Platform device inventory enforces on
// a request line ("Line exceeds limit of 8192 bytes", confirmed by wire probe).
// The chunker must keep every generated filter comfortably inside it.
const requestLineLimitBytes = 8192

func TestQuoteRSQLValue(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"FVFC41HCLYWP", `"FVFC41HCLYWP"`},
		{`with"quote`, `"with\"quote"`},
		{`with\backslash`, `"with\\backslash"`},
		{"", `""`},
	}
	for _, c := range cases {
		if got := quoteRSQLValue(c.in); got != c.want {
			t.Errorf("quoteRSQLValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestChunkSerialsByEncodedSize_SingleChunkWhenSmall(t *testing.T) {
	serials := []string{"AAAA1111", "BBBB2222", "CCCC3333"}
	chunks := chunkSerialsByEncodedSize(serials, serialFilterBudgetBytes)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0]) != len(serials) {
		t.Errorf("expected all %d serials in one chunk, got %d", len(serials), len(chunks[0]))
	}
}

// TestChunkSerialsByEncodedSize_PreservesEverySerial is the correctness contract:
// chunking must never drop or duplicate a serial, or devices go uncommanded.
func TestChunkSerialsByEncodedSize_PreservesEverySerial(t *testing.T) {
	var serials []string
	for i := range 1000 {
		serials = append(serials, fmt.Sprintf("SERIAL%06d", i))
	}

	chunks := chunkSerialsByEncodedSize(serials, serialFilterBudgetBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected 1000 serials to need multiple chunks, got %d", len(chunks))
	}

	var flat []string
	for _, c := range chunks {
		if len(c) == 0 {
			t.Error("produced an empty chunk")
		}
		flat = append(flat, c...)
	}
	if len(flat) != len(serials) {
		t.Fatalf("chunking changed the serial count: got %d, want %d", len(flat), len(serials))
	}
	for i := range serials {
		if flat[i] != serials[i] {
			t.Fatalf("order not preserved at %d: got %q, want %q", i, flat[i], serials[i])
		}
	}
}

// TestChunkSerialsByEncodedSize_EveryFilterFitsRequestLine asserts the real
// constraint end to end: each chunk's fully encoded filter, plus a generous
// allowance for the rest of the request line, stays under the server's limit.
func TestChunkSerialsByEncodedSize_EveryFilterFitsRequestLine(t *testing.T) {
	// Long serials stress the budget harder than realistic ones.
	var serials []string
	for i := range 500 {
		serials = append(serials, fmt.Sprintf("VERY-LONG-SERIAL-NUMBER-%08d", i))
	}

	// Worst-case allowance for method, path, tenant id, page and page-size.
	const requestOverheadBytes = 512

	for i, chunk := range chunkSerialsByEncodedSize(serials, serialFilterBudgetBytes) {
		quoted := make([]string, 0, len(chunk))
		for _, s := range chunk {
			quoted = append(quoted, quoteRSQLValue(s))
		}
		encoded := url.QueryEscape("serialNumber=in=(" + strings.Join(quoted, ",") + ")")
		if total := len(encoded) + requestOverheadBytes; total >= requestLineLimitBytes {
			t.Errorf("chunk %d encodes to %d bytes (+%d overhead), at or over the %d-byte limit",
				i, len(encoded), requestOverheadBytes, requestLineLimitBytes)
		}
	}
}

func TestChunkSerialsByEncodedSize_OversizedSingleSerialStillChunked(t *testing.T) {
	huge := strings.Repeat("X", serialFilterBudgetBytes*2)
	chunks := chunkSerialsByEncodedSize([]string{huge}, serialFilterBudgetBytes)
	if len(chunks) != 1 || len(chunks[0]) != 1 {
		t.Fatalf("a single oversized serial must still form one chunk, got %#v", len(chunks))
	}
}

func TestChunkSerialsByEncodedSize_Empty(t *testing.T) {
	if chunks := chunkSerialsByEncodedSize(nil, serialFilterBudgetBytes); len(chunks) != 0 {
		t.Errorf("expected no chunks for no serials, got %d", len(chunks))
	}
}

func TestSortedKeys_IsStable(t *testing.T) {
	got := sortedKeys(map[string]bool{"c": true, "a": true, "b": true})
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedKeys = %v, want %v", got, want)
		}
	}
}

// --- schema/doc guards for the batch conversion ---

// batchActions is every action that targets devices in bulk. The guards below
// run over all of them, so a new batch command action is added here and nowhere
// else.
func batchActions() map[string]action.Action {
	return map[string]action.Action{
		"send_blank_push":      NewSendBlankPushAction(),
		"enable_lost_mode":     NewEnableLostModeAction(),
		"disable_lost_mode":    NewDisableLostModeAction(),
		"play_lost_mode_sound": NewPlayLostModeSoundAction(),
	}
}

func schemaOf(t *testing.T, a action.Action) actionschema.Schema {
	t.Helper()
	var resp action.SchemaResponse
	a.Schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// TestBatchActions_UseListSelectors guards the conversion itself: every batch
// action must expose list-typed plural selectors and no singular remnant.
func TestBatchActions_UseListSelectors(t *testing.T) {
	wantType := types.ListType{ElemType: types.StringType}

	for name, a := range batchActions() {
		schema := schemaOf(t, a)

		for _, attrName := range []string{"management_ids", "serial_numbers"} {
			attr, ok := schema.Attributes[attrName]
			if !ok {
				t.Errorf("%s: missing %q", name, attrName)
				continue
			}
			if !attr.GetType().Equal(wantType) {
				t.Errorf("%s: %s must be a list of strings, got %s", name, attrName, attr.GetType())
			}
			if !attr.IsOptional() {
				t.Errorf("%s: %s must be optional (either selector may be used)", name, attrName)
			}
		}

		for _, gone := range []string{"management_id", "serial_number"} {
			if _, ok := schema.Attributes[gone]; ok {
				t.Errorf("%s: singular %q still present after the plural conversion", name, gone)
			}
		}
	}
}

// TestBatchActions_DeclareConfigValidators pairs with the schema: at-least-one-of
// cannot be expressed by the attributes alone, so an action that forgets to
// return ConfigValidators would accept a config selecting no device and only
// fail part-way through the apply.
func TestBatchActions_DeclareConfigValidators(t *testing.T) {
	for name, a := range batchActions() {
		withValidators, ok := a.(action.ActionWithConfigValidators)
		if !ok {
			t.Errorf("%s: must implement ActionWithConfigValidators", name)
			continue
		}
		if len(withValidators.ConfigValidators(context.Background())) == 0 {
			t.Errorf("%s: declares no ConfigValidators, so a config selecting no device would reach apply", name)
		}
	}
}

// TestBatchActions_DocumentBatchBehaviour is the docs-render guard: the
// single-request semantics must reach the generated documentation, since
// tfplugindocs renders descriptions but not validators.
func TestBatchActions_DocumentBatchBehaviour(t *testing.T) {
	for name, a := range batchActions() {
		desc := schemaOf(t, a).MarkdownDescription
		if !strings.Contains(desc, "single request") {
			t.Errorf("%s: description does not state that all devices are commanded in a single request", name)
		}
	}
}

// TestBatchActions_SelectorsCrossReference guards a copy-paste class of bug:
// each selector's description must point at its COUNTERPART, never at itself.
// tfplugindocs renders these verbatim, so a self-reference ships as user-facing
// nonsense that no other test would notice.
func TestBatchActions_SelectorsCrossReference(t *testing.T) {
	counterpart := map[string]string{
		"management_ids": "serial_numbers",
		"serial_numbers": "management_ids",
	}

	for name, a := range batchActions() {
		schema := schemaOf(t, a)
		for attrName, other := range counterpart {
			attr, ok := schema.Attributes[attrName]
			if !ok {
				continue
			}
			desc := attr.GetMarkdownDescription()
			if !strings.Contains(desc, "`"+other+"`") {
				t.Errorf("%s: %s description does not reference its counterpart %s", name, attrName, other)
			}
			if strings.Contains(desc, "Set this and/or `"+attrName+"`") {
				t.Errorf("%s: %s description tells the user to set the attribute in terms of itself", name, attrName)
			}
		}
	}
}

// TestBuildClientData_EveryEntryPointsAtItsOwnID is the contract buildClientData
// documents: entries hold pointers, so one shared string would command a single
// device N times and leave the rest alone while the request stayed well-formed.
func TestBuildClientData_EveryEntryPointsAtItsOwnID(t *testing.T) {
	ids := []string{"a", "b", "c"}
	clients := buildClientData(ids)
	if len(clients) != len(ids) {
		t.Fatalf("got %d entries, want %d", len(clients), len(ids))
	}
	for i, c := range clients {
		if c.ManagementID == nil || *c.ManagementID != ids[i] {
			t.Errorf("entry %d = %v, want %q", i, c.ManagementID, ids[i])
		}
	}
}

func TestBatchTargetSuffix(t *testing.T) {
	if got := batchTargetSuffix(nil); got != "" {
		t.Errorf("empty batch named devices: %q", got)
	}
	if got := batchTargetSuffix([]string{"a", "b"}); got != " (a, b)" {
		t.Errorf("small batch = %q", got)
	}
	many := make([]string, 11)
	if got := batchTargetSuffix(many); got != "" {
		t.Errorf("a batch past the naming limit must stay quiet, got %q", got)
	}
}

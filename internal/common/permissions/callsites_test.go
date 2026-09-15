// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package permissions

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// writeFixture writes src to a Go file in a fresh temporary directory and
// returns its path. Fixtures live in t.TempDir() rather than testdata/ even
// though this package already uses testdata/ for catalogue.golden: the golden is
// data, whereas these fixtures are Go source, and testdata/ is not in
// .copywrite.hcl's header_ignore list, so a .go file there would collect a
// copyright header on the next `make generate` and be swept by `make fmt`.
// Keeping the source inline also puts the forged call text beside the assertion
// about it, which is the whole point of the negative cases below.
func writeFixture(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// fixtureRegistry is the registry the fixtures are filtered against. It is
// synthetic rather than an SDK package's Privileges map so the cases below turn
// on the walk, not on which methods a given SDK release happens to publish;
// SDKCallsInFile only ever tests a name for membership, so the values are
// irrelevant here.
var fixtureRegistry = Registry{
	"GetPatchPolicyByID":     jamfplatform.MethodPrivileges{Method: "GetPatchPolicyByID"},
	"ListPatchPoliciesV2":    jamfplatform.MethodPrivileges{Method: "ListPatchPoliciesV2"},
	"UpdatePatchPolicyByID":  jamfplatform.MethodPrivileges{Method: "UpdatePatchPolicyByID"},
	"DeletePatchPolicyByID":  jamfplatform.MethodPrivileges{Method: "DeletePatchPolicyByID"},
	"SendBlankPush":          jamfplatform.MethodPrivileges{Method: "SendBlankPush"},
	"GetComputerInventoryV1": jamfplatform.MethodPrivileges{Method: "GetComputerInventoryV1"},
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestSDKCallsInFile_FindsEveryReceiverShape pins the claim SDKCallsInFile's doc
// comment makes: selector matching covers every receiver shape the provider
// uses, including a package-qualified call and a bare one-segment receiver,
// without any of them being named in the implementation.
func TestSDKCallsInFile_FindsEveryReceiverShape(t *testing.T) {
	path := writeFixture(t, `package fixture

func run(client C, r R, d D, a A) {
	client.GetPatchPolicyByID()
	r.client.UpdatePatchPolicyByID()
	d.client.DeletePatchPolicyByID()
	r.proClient.ListPatchPoliciesV2()
	a.actions.SendBlankPush()
	pro.GetComputerInventoryV1()
}
`)

	got, err := SDKCallsInFile(path, fixtureRegistry)
	if err != nil {
		t.Fatalf("SDKCallsInFile: %v", err)
	}
	want := []string{
		"DeletePatchPolicyByID",
		"GetComputerInventoryV1",
		"GetPatchPolicyByID",
		"ListPatchPoliciesV2",
		"SendBlankPush",
		"UpdatePatchPolicyByID",
	}
	if !slices.Equal(sortedKeys(got), want) {
		t.Errorf("SDKCallsInFile() = %v, want %v", sortedKeys(got), want)
	}
}

// TestSDKCallsInFile_IgnoresCommentedOutCall is the mutation the regex form
// could not survive. The construct no longer calls ListPatchPoliciesV2 — a stub
// took its place — but a leftover comment still spells the call out, in the
// three shapes a stale one turns up in: a line comment, a block comment, and a
// commented-out line of real code. The old `\b\w+\.([A-Za-z0-9]+)\(` regex
// matched all three, so the drift guard went on believing the method was called
// and the construct kept publishing its permission.
func TestSDKCallsInFile_IgnoresCommentedOutCall(t *testing.T) {
	path := writeFixture(t, `package fixture

// run used to page the collection with r.proClient.ListPatchPoliciesV2(listCtx, nil, "").
func run(r R) {
	/* formerly: r.proClient.ListPatchPoliciesV2(listCtx, nil, "") */
	// r.proClient.ListPatchPoliciesV2(listCtx, nil, "")
	r.client.GetPatchPolicyByID()
}
`)

	got, err := SDKCallsInFile(path, fixtureRegistry)
	if err != nil {
		t.Fatalf("SDKCallsInFile: %v", err)
	}
	if got["ListPatchPoliciesV2"] {
		t.Error("SDKCallsInFile() reported ListPatchPoliciesV2, which appears only in comments")
	}
	if !got["GetPatchPolicyByID"] {
		t.Error("SDKCallsInFile() missed GetPatchPolicyByID, the one call the fixture really makes")
	}
}

// TestSDKCallsInFile_IgnoresStringLiteralCall is the same mutation through the
// other hole in a byte scan: the call text sits in string literals — a log
// message, a raw string, and a declared method list of the kind permissions.go
// itself holds — and in none of them is anything called.
func TestSDKCallsInFile_IgnoresStringLiteralCall(t *testing.T) {
	path := writeFixture(t, `package fixture

var declared = []string{"ListPatchPoliciesV2("}

const usage = `+"`"+`call r.proClient.ListPatchPoliciesV2(ctx, nil, "") to page the collection`+"`"+`

func run(r R) {
	r.log("retrying r.proClient.ListPatchPoliciesV2(listCtx, nil, \"\")")
	r.client.GetPatchPolicyByID()
}
`)

	got, err := SDKCallsInFile(path, fixtureRegistry)
	if err != nil {
		t.Fatalf("SDKCallsInFile: %v", err)
	}
	if got["ListPatchPoliciesV2"] {
		t.Error("SDKCallsInFile() reported ListPatchPoliciesV2, which appears only in string literals")
	}
	if !got["GetPatchPolicyByID"] {
		t.Error("SDKCallsInFile() missed GetPatchPolicyByID, the one call the fixture really makes")
	}
}

// TestSDKCallsInFile_FiltersNonSDKCalls asserts the registry is the only filter:
// the ordinary Go calls a crud.go is full of must not reach the comparison, and
// a plausible-looking SDK name the registry does not carry must not either.
func TestSDKCallsInFile_FiltersNonSDKCalls(t *testing.T) {
	path := writeFixture(t, `package fixture

func run(r R, resp *Response) {
	resp.Diagnostics.Append(nil)
	helpers.DerefString(nil)
	r.client.GetPatchPolicyByID()
	r.client.GetPatchPolicyByIDV9000()
}
`)

	got, err := SDKCallsInFile(path, fixtureRegistry)
	if err != nil {
		t.Fatalf("SDKCallsInFile: %v", err)
	}
	if !slices.Equal(sortedKeys(got), []string{"GetPatchPolicyByID"}) {
		t.Errorf("SDKCallsInFile() = %v, want [GetPatchPolicyByID]", sortedKeys(got))
	}
}

// TestSDKCallsInFile_Errors covers the two failure modes the callers turn into
// t.Fatalf: a file that is not there and a file that does not parse. Both must
// come back as a wrapped error naming the file, because a guard that silently
// returned an empty set would report every declared method as uncalled and send
// the reader after the wrong thing.
func TestSDKCallsInFile_Errors(t *testing.T) {
	t.Run("unreadable", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent.go")
		if _, err := SDKCallsInFile(missing, fixtureRegistry); err == nil {
			t.Fatal("SDKCallsInFile() on a missing file returned no error")
		} else if !strings.Contains(err.Error(), "absent.go") {
			t.Errorf("error %q does not name the file", err)
		}
	})

	t.Run("unparseable", func(t *testing.T) {
		path := writeFixture(t, "package fixture\n\nfunc run( {\n")
		if _, err := SDKCallsInFile(path, fixtureRegistry); err == nil {
			t.Fatal("SDKCallsInFile() on unparseable source returned no error")
		} else if !strings.Contains(err.Error(), "fixture.go") {
			t.Errorf("error %q does not name the file", err)
		}
	})
}

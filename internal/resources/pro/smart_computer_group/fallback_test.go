// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"errors"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// refusal builds the shape Jamf Pro's group endpoints answer a refused
// criterion or operator with. Both wordings are the ones reproduced on
// 11.32.0 and recorded in internal/common/progroups/fallback.go.
func refusal(description string) error {
	return &jamfplatform.APIResponseError{
		StatusCode: http.StatusBadRequest,
		Method:     http.MethodPost,
		URL:        "https://example.invalid/pro/v3/computer-groups/smart-groups",
		Errors: []jamfplatform.ErrorDetail{{
			Code:        "INVALID_FIELD",
			Description: description,
		}},
	}
}

// TestClassifyWrite_OnlyARefusalTakesTheFallback pins the gate. A second write
// costs a request and gives up Jamf Pro's operator check, so it happens for the
// refusals the older group interface is known to store and for nothing else —
// not for a duplicate name, not for a refused site, not for a transport
// failure, and not for an INVALID_FIELD about something other than a criterion
// or an operator, which both paths would refuse alike.
func TestClassifyWrite_OnlyARefusalTakesTheFallback(t *testing.T) {
	cases := map[string]struct {
		err  error
		want writeOutcome
	}{
		"no error": {
			err:  nil,
			want: writeSucceeded,
		},
		"a refused patch reporting criterion": {
			err:  refusal("The criterion Patch Reporting: 1Password is not valid"),
			want: writeRefusedByJamfPro,
		},
		"a refused operator on an extension attribute": {
			err:  refusal("The operator has is not valid for extension attribute Notes"),
			want: writeRefusedByJamfPro,
		},
		"an unrelated invalid field": {
			err:  refusal("The andOr value is not valid"),
			want: writeFailed,
		},
		"a duplicate group name": {
			err: &jamfplatform.APIResponseError{
				StatusCode: http.StatusUnprocessableEntity,
				Errors: []jamfplatform.ErrorDetail{{
					Code:        "DUPLICATE_FIELD",
					Field:       "name",
					Description: "Group named x already exists",
				}},
			},
			want: writeFailed,
		},
		"a refused site": {
			err: &jamfplatform.APIResponseError{
				StatusCode: http.StatusForbidden,
				Errors: []jamfplatform.ErrorDetail{{
					Code:        "INVALID_PRIVILEGE",
					Field:       "siteId",
					Description: "Access denied: insufficient privileges",
				}},
			},
			want: writeFailed,
		},
		"a transport failure": {
			err:  errors.New("connection reset by peer"),
			want: writeFailed,
		},
	}

	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			if got := classifyWrite(tc.err); got != tc.want {
				t.Errorf("classifyWrite = %v, want %v", got, tc.want)
			}
			if refused := progroups.ModernWriteRefused(tc.err); refused != (tc.want == writeRefusedByJamfPro) {
				t.Errorf("the gate disagrees with progroups.ModernWriteRefused, which is the only thing that decides it: %v", refused)
			}
		})
	}
}

// TestBuildClassicSmartComputerGroupInput_EmitsNameSiteAndCriteria pins the
// always-emit rule on the fallback body, and that everything it carries travels
// in this one request. Jamf Pro recalculates membership synchronously with the
// write, so a body that left the criteria to a second request would publish a
// live, half-built group.
//
// Both authored priorities are dropped so that the numbering asserted below can
// only have come from the shared criterion builder, which fills it from the list
// position. criteriaPriorityValidator has already refused any authored value
// that disagrees by the time a write is built, on this path as on the other.
func TestBuildClassicSmartComputerGroupInput_EmitsNameSiteAndCriteria(t *testing.T) {
	first := criterion("Notes", "has", "urgent")
	first.Priority = types.Int64Null()
	second := criterion("Model", "like", "MacBook")
	second.Priority = types.Int64Null()
	second.AndOr = types.StringValue("or")
	second.HasClosingParenthesis = types.BoolValue(true)

	plan := SmartComputerGroupResourceModel{
		Name:     types.StringValue("placeholder group"),
		SiteID:   types.StringValue("7"),
		Criteria: []criteria.CriterionModel{first, second},
	}

	got := buildClassicSmartComputerGroupInput(plan)

	if got.Name == nil || *got.Name != "placeholder group" {
		t.Errorf("name must always be emitted, got %v", got.Name)
	}
	if got.IsSmart == nil || !*got.IsSmart {
		t.Error("the group must be written as a smart group")
	}
	if got.Site == nil || got.Site.ID == nil {
		t.Fatal("the site must always be emitted")
	}
	if *got.Site.ID != 7 {
		t.Errorf("site: expected 7, got %d", *got.Site.ID)
	}
	if got.Criteria == nil || got.Criteria.Criterion == nil {
		t.Fatal("criteria must always be emitted")
	}

	built := *got.Criteria.Criterion
	if len(built) != 2 {
		t.Fatalf("both criteria must travel in this one body, got %d", len(built))
	}
	if built[0].Name == nil || *built[0].Name != "Notes" || built[0].SearchType == nil || *built[0].SearchType != "has" {
		t.Errorf("the first criterion did not map through the shared builder: %+v", built[0])
	}
	for i, c := range built {
		if c.Priority == nil || *c.Priority != i {
			t.Errorf("criterion %d: expected priority %d, got %v", i, i, c.Priority)
		}
	}
	if built[1].AndOr == nil || *built[1].AndOr != "or" {
		t.Errorf("the authored join must survive, got %v", built[1].AndOr)
	}
	if built[1].ClosingParen == nil || !*built[1].ClosingParen {
		t.Error("the authored closing parenthesis must survive")
	}
}

// TestBuildClassicSmartComputerGroupInput_SendsNoMembership pins the one field
// that must stay absent. A smart group has no membership payload, and sending
// one answers 409 "Unable to match computer".
func TestBuildClassicSmartComputerGroupInput_SendsNoMembership(t *testing.T) {
	plan := SmartComputerGroupResourceModel{
		Name:     types.StringValue("placeholder group"),
		Criteria: []criteria.CriterionModel{criterion("Notes", "has", "urgent")},
	}

	if got := buildClassicSmartComputerGroupInput(plan); got.Computers != nil {
		t.Errorf("a smart group must send no membership collection, got %+v", got.Computers)
	}
}

// TestBuildClassicSmartComputerGroupInput_CollapsesAnUnsetSiteToTheSentinel
// pins that the no-site sentinel converts straight across: it is -1 on both
// interfaces, so a group with no site is written the same way on either path
// and nothing relies on omitting the field.
func TestBuildClassicSmartComputerGroupInput_CollapsesAnUnsetSiteToTheSentinel(t *testing.T) {
	for label, planned := range map[string]types.String{
		"an omitted site":    types.StringNull(),
		"an unknown site":    types.StringUnknown(),
		"an empty site":      types.StringValue(""),
		"the sentinel value": types.StringValue(progroups.NoSiteID),
	} {
		t.Run(label, func(t *testing.T) {
			got := buildClassicSmartComputerGroupInput(SmartComputerGroupResourceModel{
				Name:   types.StringValue("placeholder group"),
				SiteID: planned,
			})
			if got.Site == nil || got.Site.ID == nil {
				t.Fatal("the site must always be emitted")
			}
			if *got.Site.ID != -1 {
				t.Errorf("expected the no-site sentinel, got %d", *got.Site.ID)
			}
		})
	}
}

// TestBuildDescriptionPatch_CarriesNothingButTheDescription is the guard against
// the most dangerous regression in the fallback path. The same request accepts
// criteria, and a body that included them would re-run the very validation the
// fallback exists to get around — undoing, with a second refusal, the write it
// was called to complete.
func TestBuildDescriptionPatch_CarriesNothingButTheDescription(t *testing.T) {
	got := buildDescriptionPatch(types.StringValue("placeholder note"))

	if got.GroupDescription == nil || *got.GroupDescription != "placeholder note" {
		t.Errorf("description: expected %q, got %v", "placeholder note", got.GroupDescription)
	}
	if got.Criteria != nil {
		t.Errorf("the description write must never carry criteria, got %+v", got.Criteria)
	}
	if got.Assignments != nil {
		t.Errorf("the description write must never carry membership, got %+v", got.Assignments)
	}
	if got.GroupName != nil {
		t.Errorf("the description write must never carry the name, got %v", got.GroupName)
	}
}

// TestBuildDescriptionPatch_ClearsExplicitly pins that clearing a description
// sends the empty string rather than omitting the field, which on a merge patch
// would leave the stored text alone.
func TestBuildDescriptionPatch_ClearsExplicitly(t *testing.T) {
	got := buildDescriptionPatch(types.StringValue(""))
	if got.GroupDescription == nil {
		t.Fatal("a cleared description must still be emitted")
	}
	if *got.GroupDescription != "" {
		t.Errorf("expected the empty string, got %q", *got.GroupDescription)
	}
}

// TestDescriptionNeedsWrite pins when the fallback update issues its second
// request. The older group interface preserves a description it cannot express,
// so an update that does not change the description must stay a single write —
// which is what keeps an ordinary criteria-only edit from touching the group
// twice.
func TestDescriptionNeedsWrite(t *testing.T) {
	cases := map[string]struct {
		planned types.String
		prior   types.String
		want    bool
	}{
		"an unchanged note": {
			planned: types.StringValue("placeholder note"),
			prior:   types.StringValue("placeholder note"),
			want:    false,
		},
		"an attribute dropped from the configuration": {
			planned: types.StringUnknown(),
			prior:   types.StringValue(""),
			want:    false,
		},
		"both unset": {
			planned: types.StringNull(),
			prior:   types.StringValue(""),
			want:    false,
		},
		"an edited note": {
			planned: types.StringValue("placeholder note, edited"),
			prior:   types.StringValue("placeholder note"),
			want:    true,
		},
		"a note added to a group without one": {
			planned: types.StringValue("placeholder note"),
			prior:   types.StringValue(""),
			want:    true,
		},
		"a note cleared": {
			planned: types.StringValue(""),
			prior:   types.StringValue("placeholder note"),
			want:    true,
		},
	}

	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			if got := descriptionNeedsWrite(tc.planned, tc.prior); got != tc.want {
				t.Errorf("descriptionNeedsWrite = %v, want %v", got, tc.want)
			}
		})
	}
}

// classicRefusal builds the shape Jamf Pro's older group interface answers a
// refused write with: an HTML status page, whose message the SDK lifts into a
// detail carrying no code at all, which is why the classifier matches the
// wording.
func classicRefusal(status int, description string) error {
	return &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     http.MethodPost,
		URL:        "https://example.invalid/JSSResource/computergroups/id/0",
		Errors: []jamfplatform.ErrorDetail{{
			Description: description,
		}},
	}
}

// TestClassicRefusedCriteria pins which second failure is about the criteria.
// The older interface validates criterion names and nothing else, so its
// "Problem with criteria" is the one refusal BothRefusedDiagnostics describes;
// a name already in use, a refused site and a transport failure all reach the
// same code path and none of them is a criteria problem.
func TestClassicRefusedCriteria(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"the older interface refusing the criteria": {
			err:  classicRefusal(http.StatusConflict, "Problem with criteria"),
			want: true,
		},
		"the same refusal in another case": {
			err:  classicRefusal(http.StatusConflict, "PROBLEM WITH CRITERIA"),
			want: true,
		},
		"a duplicate group name": {
			err:  classicRefusal(http.StatusConflict, "Problem with name"),
			want: false,
		},
		"a refused site": {
			err:  classicRefusal(http.StatusConflict, "Problem with site ID"),
			want: false,
		},
		"a gateway failure": {
			err:  classicRefusal(http.StatusBadGateway, "Bad Gateway"),
			want: false,
		},
		"a transport failure": {
			err:  errors.New("connection reset by peer"),
			want: false,
		},
		"no error at all": {
			err:  nil,
			want: false,
		},
	}

	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			if got := progroups.OlderInterfaceRefusedCriteria(tc.err); got != tc.want {
				t.Errorf("classicRefusedCriteria = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFallbackWriteDiagnostics_BlamesTheCriteriaOnlyWhenBothRefusedThem is the
// guard against the diagnosis that cannot be acted on. A group whose criteria
// the group endpoint refuses and whose name is already in use fails on the
// second write for the name, and telling the operator their criteria are wrong
// points them at the one thing editing which cannot help.
func TestFallbackWriteDiagnostics_BlamesTheCriteriaOnlyWhenBothRefusedThem(t *testing.T) {
	modernErr := refusal("The criterion Operating Sistem Version is not valid")

	t.Run("both refused the criteria", func(t *testing.T) {
		got := progroups.FallbackWriteDiagnostics(groupLabel, path.Root("site_id"), path.Empty(), modernErr, classicRefusal(http.StatusConflict, "Problem with criteria"))
		if len(got) != 1 || !got.HasError() {
			t.Fatalf("expected the both-refused error on its own, got %v", got)
		}
		if !strings.Contains(got[0].Summary(), "criteria") {
			t.Errorf("the criteria must be named, got %q", got[0].Summary())
		}
		if !strings.Contains(got[0].Detail(), "Operating Sistem Version") {
			t.Errorf("the group endpoint's message names the offending criterion and must lead, got %q", got[0].Detail())
		}
	})

	t.Run("the second write failed for the name", func(t *testing.T) {
		duplicate := &jamfplatform.APIResponseError{
			StatusCode: http.StatusConflict,
			Errors: []jamfplatform.ErrorDetail{{
				Code:        "DUPLICATE_FIELD",
				Field:       "name",
				Description: "Group named x already exists",
			}},
		}
		got := progroups.FallbackWriteDiagnostics(groupLabel, path.Root("site_id"), path.Empty(), modernErr, duplicate)
		if !got.HasError() {
			t.Fatalf("a failed write must still fail the apply, got %v", got)
		}
		var errorSummaries []string
		for _, d := range got {
			if d.Severity() == diag.SeverityError {
				errorSummaries = append(errorSummaries, d.Summary())
			}
		}
		if len(errorSummaries) != 1 {
			t.Fatalf("expected exactly one error, got %v", errorSummaries)
		}
		if strings.Contains(errorSummaries[0], "criteria") {
			t.Errorf("the failure was the name, so the error must not blame the criteria: %q", errorSummaries[0])
		}
		if !strings.Contains(errorSummaries[0], "name") {
			t.Errorf("expected the duplicate-name translation, got %q", errorSummaries[0])
		}
		if got.WarningsCount() != 1 {
			t.Fatalf("the refusal that caused the second write must be reported, got %v", got.Warnings())
		}
		if !strings.Contains(got.Warnings()[0].Detail(), "Operating Sistem Version") {
			t.Errorf("the warning must carry what Jamf Pro said about the criteria, got %q", got.Warnings()[0].Detail())
		}
	})

	t.Run("the second write failed in transport", func(t *testing.T) {
		got := progroups.FallbackWriteDiagnostics(groupLabel, path.Root("site_id"), path.Empty(), modernErr, errors.New("connection reset by peer"))
		if !got.HasError() {
			t.Fatalf("a failed write must still fail the apply, got %v", got)
		}
		for _, d := range got.Errors() {
			if strings.Contains(d.Summary(), "criteria") {
				t.Errorf("a transport failure is not a criteria problem: %q", d.Summary())
			}
		}
	})
}

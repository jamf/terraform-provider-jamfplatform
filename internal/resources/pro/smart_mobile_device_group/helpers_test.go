// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
)

// These cover the one plan shape the resource model cannot represent: a criteria
// list Terraform has not resolved yet. The framework's reflection refuses an
// unknown value into the model's Go slice and aborts the plan with "This is
// always an error in the provider", so the guard is what keeps the resource
// usable from a module whose criteria come from something created in the same
// apply.

// TestCriteriaAtPlanTime_DefersOnAnUnknownList is the regression: the whole list
// is unknown, and the reader must report it as unsettled rather than decode it.
func TestCriteriaAtPlanTime_DefersOnAnUnknownList(t *testing.T) {
	plan := planWithCriteria(t, unknownCriteriaValue(t))

	models, settled, diags := criteriaAtPlanTime(t.Context(), plan)
	if diags.HasError() {
		t.Fatalf("an unknown criteria list must not raise a diagnostic, got %v", diags)
	}
	if settled {
		t.Error("an unknown criteria list must read as unsettled")
	}
	if models != nil {
		t.Errorf("an unknown criteria list must decode to nothing, got %v", models)
	}
}

// TestCriteriaAtPlanTime_DefersOnAnUnknownElement covers the narrower shape: the
// list is known but one element is not, which a for_each over an unresolved map
// produces and which reflection refuses into the criterion struct.
func TestCriteriaAtPlanTime_DefersOnAnUnknownElement(t *testing.T) {
	listType := criteriaAttributeType(t)
	plan := planWithCriteria(t, tftypes.NewValue(listType, []tftypes.Value{
		tftypes.NewValue(listType.ElementType, tftypes.UnknownValue),
	}))

	_, settled, diags := criteriaAtPlanTime(t.Context(), plan)
	if diags.HasError() {
		t.Fatalf("an unknown criterion must not raise a diagnostic, got %v", diags)
	}
	if settled {
		t.Error("a list holding an unknown criterion must read as unsettled")
	}
}

// TestCriteriaAtPlanTime_ReadsASettledList is the ordinary case, and it pins
// that the positional read returns what a decode of the model would have.
func TestCriteriaAtPlanTime_ReadsASettledList(t *testing.T) {
	authored := []criteria.CriterionModel{
		criterion("Model", "like", "iPad"),
		criterion("Supervised", "is", "true"),
	}
	plan := planWithCriteria(t, criteriaTerraformValue(t, authored))

	models, settled, diags := criteriaAtPlanTime(t.Context(), plan)
	if diags.HasError() {
		t.Fatalf("a settled criteria list must not raise a diagnostic, got %v", diags)
	}
	if !settled {
		t.Fatal("a settled criteria list must read as settled")
	}
	if criterionListsDiffer(models, authored) {
		t.Errorf("the read must return the authored criteria, got %v", models)
	}
}

// TestCriteriaAtPlanTime_TreatsAnOmittedListAsSettled keeps the "group with no
// criteria" configuration out of the deferral path: nothing is pending there,
// the list is simply absent.
func TestCriteriaAtPlanTime_TreatsAnOmittedListAsSettled(t *testing.T) {
	plan := planWithCriteria(t, tftypes.NewValue(criteriaAttributeType(t), nil))

	models, settled, diags := criteriaAtPlanTime(t.Context(), plan)
	if diags.HasError() {
		t.Fatalf("an omitted criteria list must not raise a diagnostic, got %v", diags)
	}
	if !settled {
		t.Error("an omitted criteria list must read as settled")
	}
	if len(models) != 0 {
		t.Errorf("an omitted criteria list must decode to no criteria, got %v", models)
	}
}

// TestModifyPlan_DefersOnAnUnknownCriteriaList exercises the wiring rather than
// the reader: an update whose planned criteria are unknown must leave the plan
// alone and raise nothing, where decoding the model would have failed the plan.
func TestModifyPlan_DefersOnAnUnknownCriteriaList(t *testing.T) {
	r := &SmartMobileDeviceGroupResource{ldap: unsearchedDirectory{t: t}}
	plan := planWithCriteria(t, unknownCriteriaValue(t))
	state := stateWithCriteria(t, criteriaTerraformValue(t, []criteria.CriterionModel{criterion("Model", "like", "iPad")}))

	resp := &resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(t.Context(), resource.ModifyPlanRequest{Plan: plan, State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an unknown criteria list must plan cleanly, got %v", resp.Diagnostics)
	}
	if !resp.Plan.Raw.Equal(plan.Raw) {
		t.Error("an unknown criteria list must leave the plan untouched")
	}
}

// TestModifyPlan_LeavesANonDirectoryPlanAlone is the settled counterpart: a plan
// whose criteria name no directory-service group has nothing to suppress, so the
// planned value must round-trip and the directory must never be consulted.
func TestModifyPlan_LeavesANonDirectoryPlanAlone(t *testing.T) {
	r := &SmartMobileDeviceGroupResource{ldap: unsearchedDirectory{t: t}}
	plan := planWithCriteria(t, criteriaTerraformValue(t, []criteria.CriterionModel{criterion("Model", "like", "iPhone")}))
	state := stateWithCriteria(t, criteriaTerraformValue(t, []criteria.CriterionModel{criterion("Model", "like", "iPad")}))

	resp := &resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(t.Context(), resource.ModifyPlanRequest{Plan: plan, State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a plan with no directory criterion must plan cleanly, got %v", resp.Diagnostics)
	}
	if !resp.Plan.Raw.Equal(plan.Raw) {
		t.Error("a plan with nothing to suppress must round-trip untouched")
	}
}

// resourceSchemaObjectType returns the resource's whole object type, so the plan
// values below are assembled from the live schema and cannot drift from it.
func resourceSchemaObjectType(t *testing.T) tftypes.Object {
	t.Helper()
	var resp resource.SchemaResponse
	(&SmartMobileDeviceGroupResource{}).Schema(t.Context(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("the schema must build cleanly, got %v", resp.Diagnostics)
	}
	objectType, ok := resp.Schema.Type().TerraformType(t.Context()).(tftypes.Object)
	if !ok {
		t.Fatal("the resource schema must be an object")
	}
	return objectType
}

// criteriaAttributeType returns the `criteria` attribute's own Terraform type.
func criteriaAttributeType(t *testing.T) tftypes.List {
	t.Helper()
	listType, ok := resourceSchemaObjectType(t).AttributeTypes["criteria"].(tftypes.List)
	if !ok {
		t.Fatal("criteria must be a list")
	}
	return listType
}

// unknownCriteriaValue is the value a configuration deriving the list from an
// unresolved source presents at plan.
func unknownCriteriaValue(t *testing.T) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(criteriaAttributeType(t), tftypes.UnknownValue)
}

// criteriaTerraformValue renders criterion models as the attribute's Terraform
// value, through the same conversion the resource uses.
func criteriaTerraformValue(t *testing.T, models []criteria.CriterionModel) tftypes.Value {
	t.Helper()
	list, diags := criteria.CriteriaListValue(t.Context(), models)
	if diags.HasError() {
		t.Fatalf("the criteria must convert cleanly, got %v", diags)
	}
	value, err := list.ToTerraformValue(t.Context())
	if err != nil {
		t.Fatalf("the criteria must render as a Terraform value: %v", err)
	}
	return value
}

// rawResourceValue builds a whole resource object carrying criteriaValue, with
// every other attribute unknown, which is enough for a reader that touches one
// attribute and for the plan-untouched comparisons above.
func rawResourceValue(t *testing.T, criteriaValue tftypes.Value) tftypes.Value {
	t.Helper()
	objectType := resourceSchemaObjectType(t)
	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attributeType, tftypes.UnknownValue)
	}
	attributes["criteria"] = criteriaValue
	return tftypes.NewValue(objectType, attributes)
}

// planWithCriteria returns a plan whose criteria attribute is criteriaValue.
func planWithCriteria(t *testing.T, criteriaValue tftypes.Value) tfsdk.Plan {
	t.Helper()
	var resp resource.SchemaResponse
	(&SmartMobileDeviceGroupResource{}).Schema(t.Context(), resource.SchemaRequest{}, &resp)
	return tfsdk.Plan{Schema: resp.Schema, Raw: rawResourceValue(t, criteriaValue)}
}

// stateWithCriteria returns prior state whose criteria attribute is
// criteriaValue.
func stateWithCriteria(t *testing.T, criteriaValue tftypes.Value) tfsdk.State {
	t.Helper()
	var resp resource.SchemaResponse
	(&SmartMobileDeviceGroupResource{}).Schema(t.Context(), resource.SchemaRequest{}, &resp)
	return tfsdk.State{Schema: resp.Schema, Raw: rawResourceValue(t, criteriaValue)}
}

// unsearchedDirectory fails the test if the suppression reaches the directory.
// Neither case above should: one defers before comparing anything, and the other
// names no directory-service group criterion.
type unsearchedDirectory struct{ t *testing.T }

func (d unsearchedDirectory) SearchLdapGroupsV1(context.Context, string) (*pro.LdapGroupSearchResults, error) {
	d.t.Error("suppression must not search the directory for a non-directory criterion")
	return nil, nil
}

// A create that returns an error alongside state leaves Terraform holding the
// object as tainted, and a tainted object is planned for replacement rather than
// for repair, where the same failure on an update leaves an ordinary managed
// object the next apply can finish. So the two recoveries are different
// instructions, and the point of these is that they cannot be collapsed back
// into one message that is wrong on one of the paths.

// TestDescriptionNotSetAfterCreate_NamesTheTaintRecovery pins the create half:
// the message has to say the object is tainted and name untaint, because "apply
// again to finish the change" describes a repair Terraform does not perform.
func TestDescriptionNotSetAfterCreate_NamesTheTaintRecovery(t *testing.T) {
	detail := descriptionNotSetAfterCreate("smart mobile device group", errPlatformIDUnresolved).Detail()

	for _, want := range []string{"tainted", "terraform untaint", "replaces it"} {
		if !strings.Contains(detail, want) {
			t.Errorf("create diagnostic does not mention %q:\n%s", want, detail)
		}
	}
}

// TestDescriptionNotSetAfterUpdate_PromisesTheNextApply pins the update half,
// where the group stays managed and the second write is genuinely retried.
func TestDescriptionNotSetAfterUpdate_PromisesTheNextApply(t *testing.T) {
	detail := descriptionNotSetAfterUpdate("smart mobile device group", errPlatformIDUnresolved).Detail()

	if !strings.Contains(detail, "apply again to finish it") {
		t.Errorf("update diagnostic does not promise the next apply:\n%s", detail)
	}
	if strings.Contains(detail, "tainted") {
		t.Errorf("update diagnostic claims the object is tainted, which only a create does:\n%s", detail)
	}
}

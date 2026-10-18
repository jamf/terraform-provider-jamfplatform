// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	uncheckedLegacyPayloadSummary = "Legacy payload was not checked against Apple's schemas"
	uncheckedDeclarationSummary   = "Declaration payload was not checked against Apple's schemas"
)

// uncheckedPayloadWarnings warns about every legacy payload and declaration whose content is known
// only at apply, so the plan-time check against Apple's schemas never ran for it.
//
// It belongs in ModifyPlan rather than in the schema validators. Terraform runs the validators twice
// during a plan: first while validating, with module variables and each.key or each.value still
// unknown, and again while planning, with them resolved. A validator cannot tell the two calls apart,
// so it skips an unknown silently and leaves the second call to check it. ModifyPlan runs only while
// planning, so a value still unknown here stays unknown until the apply, and the apply path writes a
// payload without validating it. GitHub issue #484 is the report that started this: a payload built
// from a module variable failed every plan, because the first call read the unknown settings as empty.
//
// A warning rather than an error, because the configuration may be valid. The platform still rejects
// a wrong value type or a missing required key at apply. What nothing reports is an unrecognised key,
// which the platform stores, reports as a success and never delivers.
//
// The configuration is read with GetAttribute, value by value, because Config.Get fails outright on
// an unknown nested collection, and an unknown is exactly what this looks for.
func uncheckedPayloadWarnings(ctx context.Context, config tfsdk.Config) diag.Diagnostics {
	var diags diag.Diagnostics

	blocksPath := path.Root("component_blocks")
	var blocks types.List
	if d := config.GetAttribute(ctx, blocksPath, &blocks); d.HasError() {
		return diags
	}
	if blocks.IsUnknown() {
		diags.AddAttributeWarning(blocksPath, uncheckedLegacyPayloadSummary,
			"Terraform will not know these component blocks until apply, so the provider cannot check any "+
				"legacy payload or declaration in them against Apple's schemas during plan. "+uncheckedConsequence)
	}
	for i, element := range blocks.Elements() {
		blockPath := blocksPath.AtListIndex(i)
		block, ok := element.(types.Object)
		if !ok {
			continue
		}
		if block.IsUnknown() {
			diags.AddAttributeWarning(blockPath, uncheckedLegacyPayloadSummary,
				"Terraform will not know this component block until apply, so the provider cannot check any "+
					"legacy payload or declaration in it against Apple's schemas during plan. "+uncheckedConsequence)
			continue
		}
		attrs := block.Attributes()
		appendUncheckedElements(&diags, attrs["legacy_payloads"], blockPath.AtName("legacy_payloads"), "payload_type", "settings", false)
		appendUncheckedElements(&diags, attrs["apple_declarations"], blockPath.AtName("apple_declarations"), "type", "payload", true)
		appendUncheckedCustomDeclarations(&diags, attrs["custom_declarations"], blockPath.AtName("custom_declarations"))
	}

	var custom types.Object
	if d := config.GetAttribute(ctx, path.Root("custom_declarations"), &custom); !d.HasError() {
		appendUncheckedCustomDeclarations(&diags, custom, path.Root("custom_declarations"))
	}

	var flat types.Dynamic
	if d := config.GetAttribute(ctx, path.Root("legacy_payloads"), &flat); !d.HasError() && !flat.IsNull() && !isFullyKnown(ctx, flat) {
		diags.AddAttributeWarning(path.Root("legacy_payloads"), uncheckedLegacyPayloadSummary,
			"Terraform will not know part of legacy_payloads until apply, so the provider cannot check any "+
				"payload in it against Apple's schemas during plan. "+uncheckedConsequence)
	}

	return diags
}

// uncheckedConsequence closes every warning with what the skipped check would have caught and how to
// get it back.
const uncheckedConsequence = "Jamf Pro refuses a wrong value type or a missing required setting when " +
	"the apply writes it. It accepts an unrecognised key, reports success and never delivers it. To have " +
	"the plan check it, build it from values Terraform knows at plan time."

// appendUncheckedElements warns about each element of a legacy payload or declaration list whose
// type or content is unknown. A whole collection still unknown gets one warning on the collection.
func appendUncheckedElements(diags *diag.Diagnostics, collection attr.Value, at path.Path, typeName, contentName string, declaration bool) {
	summary, noun := uncheckedLegacyPayloadSummary, "legacy payload"
	if declaration {
		summary, noun = uncheckedDeclarationSummary, "declaration"
	}

	list, ok := collection.(types.List)
	if !ok || list.IsNull() {
		return
	}
	if list.IsUnknown() {
		diags.AddAttributeWarning(at, summary,
			"Terraform will not know this list until apply, so the provider cannot check any "+noun+
				" in it against Apple's schemas during plan. "+uncheckedConsequence)
		return
	}
	for i, element := range list.Elements() {
		if !elementUnchecked(element, typeName, contentName) {
			continue
		}
		diags.AddAttributeWarning(at.AtListIndex(i), summary,
			"Terraform will not know this "+noun+" until apply, so the provider cannot check it against "+
				"Apple's schemas during plan. "+uncheckedConsequence)
	}
}

// appendUncheckedCustomDeclarations warns about each custom declaration whose type or payload is
// unknown. The declarations are a set, which has no stable index, so a warning lands on the set and
// names the declaration type when it is known, as declarationSchemaValidator.ValidateObject does.
func appendUncheckedCustomDeclarations(diags *diag.Diagnostics, component attr.Value, at path.Path) {
	object, ok := component.(types.Object)
	if !ok || object.IsNull() {
		return
	}
	at = at.AtName("declaration")
	if object.IsUnknown() {
		diags.AddAttributeWarning(at, uncheckedDeclarationSummary,
			"Terraform will not know these custom declarations until apply, so the provider cannot check "+
				"them against Apple's schemas during plan. "+uncheckedConsequence)
		return
	}

	set, ok := object.Attributes()["declaration"].(types.Set)
	if !ok || set.IsNull() {
		return
	}
	if set.IsUnknown() {
		diags.AddAttributeWarning(at, uncheckedDeclarationSummary,
			"Terraform will not know these custom declarations until apply, so the provider cannot check "+
				"them against Apple's schemas during plan. "+uncheckedConsequence)
		return
	}
	for _, element := range set.Elements() {
		if !elementUnchecked(element, "type", "payload") {
			continue
		}
		attribution := ""
		if object, ok := element.(types.Object); ok {
			if declarationType, ok := object.Attributes()["type"].(types.String); ok && !declarationType.IsUnknown() {
				attribution = declarationType.ValueString()
			}
		}
		diags.AddAttributeWarning(at, uncheckedDeclarationSummary,
			attributionPrefix(attribution)+"Terraform will not know this payload until apply, so the "+
				"provider cannot check it against Apple's schemas during plan. "+uncheckedConsequence)
	}
}

// elementUnchecked reports whether a payload or declaration element is unknown, or carries an unknown
// type or content, which are the two values the schema check reads.
func elementUnchecked(element attr.Value, typeName, contentName string) bool {
	object, ok := element.(types.Object)
	if !ok {
		return false
	}
	if object.IsUnknown() {
		return true
	}
	attrs := object.Attributes()
	return attrs[typeName].IsUnknown() || attrs[contentName].IsUnknown()
}

// isFullyKnown reports whether a value holds no unknown at any depth. A value that cannot be converted
// is reported as known, leaving it to the check that reads it to report what is wrong with it.
func isFullyKnown(ctx context.Context, value attr.Value) bool {
	raw, err := value.ToTerraformValue(ctx)
	if err != nil {
		return true
	}
	return raw.IsFullyKnown()
}

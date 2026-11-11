// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ resource.ResourceWithUpgradeState = &PolicyResource{}

// UpgradeState migrates prior state versions to the current schema.
//
// Both migrations rewrite the raw prior-state JSON and re-emit it through the
// current schema type. Every attribute they do not name is carried across
// verbatim as raw JSON, so no value is reinterpreted or lost.
//
// v0 → v1: the script-assignment parameter attributes were renamed from
// `parameter4`…`parameter11` to snake_case `parameter_4`…`parameter_11` to
// align with the jamfplatform_pro_script resource and the provider-wide
// snake_case attribute convention. A nested-object attribute rename is
// something the framework cannot decode against the newer schema on its own.
//
// v1 → v2: two dead attributes were removed, and an attribute gone from the
// schema must be gone from prior state too or the decode fails on the surplus
// key. `printers.leave_existing_default` modelled a dead wire element — Jamf
// Pro answers `<leave_existing_default/>` however the field was written, on an
// admin-UI-configured policy as much as a Terraform-managed one, and the
// default-printer choice it appeared to express is persisted per printer as
// `printers[].make_default`. `management_account.managed_password_length` was
// the same shape: never returned, never parsed (a length of "abc" is accepted
// as readily as 16), and with no action for a generated-password length to
// apply to, the enum being rotate or doNotChange.
//
// v2 → v3: script_assignments, printer_assignments, dock_item_assignments,
// directory_bindings, packages.packages and the deprecated wrappers'
// scripts.scripts, printers.printers and dock_items.dock_items became lists,
// because Jamf Pro stores a repeated entry and a set cannot hold one (issue
// #568). The element types are unchanged and a set and a list both serialise
// as a JSON array, so the upgrader rewrites nothing: the stored order is kept
// verbatim, and the first refresh realigns it. The bump exists for two reasons.
// It records that v3 state is decoded as lists, from a version known to have
// written the same elements as sets. And it stops a provider still on v2 from
// reading v3 state, which can hold a repeated entry its sets cannot decode:
// Terraform refuses state written by a newer schema version rather than
// letting the older provider fail on it.
//
// The v0 and v1 upgraders apply every later rewrite too, because state written
// against them has never been through the later steps. Terraform runs exactly
// one upgrader — the one matching the stored version — rather than chaining
// them.
func (r *PolicyResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				r.upgradeRawState(ctx, req, resp, "v0", func(raw []byte) ([]byte, error) {
					renamed, err := renameScriptParameterKeys(raw)
					if err != nil {
						return nil, fmt.Errorf("renaming script parameter keys: %w", err)
					}
					return dropRemovedV2Attributes(renamed)
				})
			},
		},
		1: {
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				r.upgradeRawState(ctx, req, resp, "v1", dropRemovedV2Attributes)
			},
		},
		2: {
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				r.upgradeRawState(ctx, req, resp, "v2", keepRawState)
			},
		},
	}
}

// upgradeRawState applies one raw-JSON rewrite and re-encodes the result
// against the current schema. from names the stored version for diagnostics.
func (r *PolicyResource) upgradeRawState(
	ctx context.Context,
	req resource.UpgradeStateRequest,
	resp *resource.UpgradeStateResponse,
	from string,
	rewrite func([]byte) ([]byte, error),
) {
	if req.RawState == nil {
		return
	}

	fail := func(what string, err error) {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to upgrade jamfplatform_pro_policy state from %s", from),
			fmt.Sprintf("%s: %s", what, helpers.APIErrorDetail(err)),
		)
	}

	rewritten, err := rewrite(req.RawState.JSON)
	if err != nil {
		fail("Could not rewrite prior state", err)
		return
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)

	upgraded := tfprotov6.RawState{JSON: rewritten}
	value, err := upgraded.Unmarshal(schemaType)
	if err != nil {
		fail("Could not decode rewritten prior state against the current schema", err)
		return
	}

	dynamicValue, err := tfprotov6.NewDynamicValue(schemaType, value)
	if err != nil {
		fail("Could not encode upgraded state", err)
		return
	}

	resp.DynamicValue = &dynamicValue
}

// keepRawState is the v2 → v3 rewrite, which changes nothing; see
// UpgradeState.
func keepRawState(rawJSON []byte) ([]byte, error) {
	return rawJSON, nil
}

// dropRemovedV2Attributes removes the two attributes v2 deleted from
// prior-state JSON: printers.leave_existing_default and
// management_account.managed_password_length. Everything else is preserved as
// raw JSON bytes, so numeric IDs and other values keep their exact
// representation. A state that never carried a key, or carries a null parent
// block, is returned untouched.
func dropRemovedV2Attributes(rawJSON []byte) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(rawJSON, &top); err != nil {
		return nil, err
	}

	changed := false
	for _, removed := range []struct{ block, key string }{
		{"printers", "leave_existing_default"},
		{"management_account", "managed_password_length"},
	} {
		blockRaw, ok := top[removed.block]
		if !ok || string(blockRaw) == "null" {
			continue
		}
		var block map[string]json.RawMessage
		if err := json.Unmarshal(blockRaw, &block); err != nil {
			return nil, err
		}
		if _, present := block[removed.key]; !present {
			continue
		}
		delete(block, removed.key)
		rewritten, err := json.Marshal(block)
		if err != nil {
			return nil, err
		}
		top[removed.block] = rewritten
		changed = true
	}
	if !changed {
		return rawJSON, nil
	}

	return json.Marshal(top)
}

// renameScriptParameterKeys rewrites the prior-state JSON, renaming each
// scripts.scripts[*].parameterN key to parameter_N (N = 4..11). All other
// state is preserved as raw JSON bytes — only the eight affected keys are
// touched — so numeric IDs and other values keep their exact representation.
func renameScriptParameterKeys(rawJSON []byte) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(rawJSON, &top); err != nil {
		return nil, err
	}

	scriptsRaw, ok := top["scripts"]
	if !ok || string(scriptsRaw) == "null" {
		return rawJSON, nil
	}

	var scriptsBlock map[string]json.RawMessage
	if err := json.Unmarshal(scriptsRaw, &scriptsBlock); err != nil {
		return nil, err
	}

	itemsRaw, ok := scriptsBlock["scripts"]
	if !ok || string(itemsRaw) == "null" {
		return rawJSON, nil
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return nil, err
	}

	for _, item := range items {
		for n := 4; n <= 11; n++ {
			oldKey := fmt.Sprintf("parameter%d", n)
			value, present := item[oldKey]
			if !present {
				continue
			}
			item[fmt.Sprintf("parameter_%d", n)] = value
			delete(item, oldKey)
		}
	}

	newItems, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	scriptsBlock["scripts"] = newItems

	newScripts, err := json.Marshal(scriptsBlock)
	if err != nil {
		return nil, err
	}
	top["scripts"] = newScripts

	return json.Marshal(top)
}

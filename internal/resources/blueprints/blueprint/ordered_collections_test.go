// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func str(v string) types.String { return types.StringValue(v) }

// orderedFixtureBlock returns one component block whose every ordered collection holds an
// identical repeat, a pair differing only in a secondary field, and entries out of any sorted
// order. reversed reverses every collection at every level, which is what a reorder in
// configuration looks like.
func orderedFixtureBlock(reversed bool) ComponentBlockModel {
	order := func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		if reversed {
			slices.Reverse(out)
		}
		return out
	}
	pick := func(all []any) []any {
		out := make([]any, 0, len(all))
		for _, i := range order(len(all)) {
			out = append(out, all[i])
		}
		return out
	}
	typed := func(all []any, into any) {
		v := reflect.ValueOf(into).Elem()
		for _, e := range pick(all) {
			v.Set(reflect.Append(v, reflect.ValueOf(e)))
		}
	}

	bm := func(title, url string) any {
		return components.BookmarkModel{Type: str("bookmark"), Title: str(title), URL: str(url)}
	}
	var folder []components.UrlBookmarkModel
	typed([]any{
		components.UrlBookmarkModel{Title: str("m"), URL: str("https://m.example.com")},
		components.UrlBookmarkModel{Title: str("b"), URL: str("https://b.example.com")},
		components.UrlBookmarkModel{Title: str("b"), URL: str("https://b.example.com")},
	}, &folder)
	var work []components.BookmarkModel
	typed([]any{
		bm("Z", "https://z.example.com"), bm("A", "https://a.example.com"),
		bm("A", "https://a.example.com"), bm("A", "https://a2.example.com"),
		components.BookmarkModel{Type: str("folder"), Title: str("F"), Folder: folder},
	}, &work)
	single := []components.BookmarkModel{{Type: str("bookmark"), Title: str("Z"), URL: str("https://z.example.com")}}
	var groups []components.BookmarkGroupModel
	typed([]any{
		components.BookmarkGroupModel{GroupIdentifier: str("g1"), Title: str("Work"), Bookmarks: work},
		components.BookmarkGroupModel{GroupIdentifier: str("g1"), Title: str("Work"), Bookmarks: single},
		components.BookmarkGroupModel{GroupIdentifier: str("g1"), Title: str("Work"), Bookmarks: single},
		components.BookmarkGroupModel{GroupIdentifier: str("g0"), Title: str("Home"), Bookmarks: single},
	}, &groups)

	domain := func(d string) any { return components.ManagedExtensionDomainModel{Domain: str(d)} }
	var allowed, denied []components.ManagedExtensionDomainModel
	typed([]any{domain("z.example.com"), domain("a.example.com"), domain("a.example.com"), domain("m.example.com")}, &allowed)
	typed([]any{domain("d.example.com"), domain("d.example.com"), domain("c.example.com")}, &denied)

	launchd := func(ctx, url string) any {
		return components.LaunchdItemModel{Context: str(ctx), FileAssetReference: &components.DataAssetRefModel{
			DataURL: str(url), HashSHA256: str(hashA), ContentType: str("application/x-plist"),
		}}
	}
	var launchds []components.LaunchdItemModel
	typed([]any{
		launchd("daemon", "https://e.example.com/2.plist"), launchd("daemon", "https://e.example.com/1.plist"),
		launchd("daemon", "https://e.example.com/1.plist"), launchd("agent", "https://e.example.com/1.plist"),
	}, &launchds)
	exe := func(url, hash string) *components.DataAssetRefModel {
		return &components.DataAssetRefModel{DataURL: str(url), HashSHA256: str(hash), ContentType: str("application/zip")}
	}
	var tasks []components.ServiceBackgroundTaskModel
	typed([]any{
		components.ServiceBackgroundTaskModel{TaskType: str("com.example.z"), TaskDescription: str("Z"), ExecutableAssetReference: exe("https://e.example.com/z.zip", hashA), LaunchdConfigurations: launchds},
		components.ServiceBackgroundTaskModel{TaskType: str("com.example.a"), TaskDescription: str("A"), ExecutableAssetReference: exe("https://e.example.com/a.zip", hashB)},
		components.ServiceBackgroundTaskModel{TaskType: str("com.example.a"), TaskDescription: str("A"), ExecutableAssetReference: exe("https://e.example.com/a.zip", hashB)},
		components.ServiceBackgroundTaskModel{TaskType: str("com.example.a"), TaskDescription: str("A2"), ExecutableAssetReference: exe("https://e.example.com/a.zip", hashB)},
	}, &tasks)

	file := func(service, url string) any {
		return components.ServiceConfigFileModel{ServiceType: str(service), DataAssetReference: &components.ServiceConfigDataAssetRefModel{
			DataURL: str(url), HashSHA256: str(hashB), ContentType: str("application/zip"),
		}}
	}
	var files []components.ServiceConfigFileModel
	typed([]any{
		file("com.apple.sshd", "https://e.example.com/z.zip"), file("com.apple.bash", "https://e.example.com/a.zip"),
		file("com.apple.bash", "https://e.example.com/a.zip"), file("com.apple.bash", "https://e.example.com/b.zip"),
	}, &files)

	program := func(token, description string) any {
		return components.BetaProgramModel{Token: str(token), Description: str(description)}
	}
	var programs []components.BetaProgramModel
	typed([]any{program("tok-z", "Zed"), program("tok-a", "Ay"), program("tok-a", "Ay"), program("tok-a", "Ay two")}, &programs)

	declaration := func(declarationType, payload string) any {
		return components.CustomDeclarationModel{ChannelType: str("SYSTEM"), Kind: str("CONFIGURATION"), Type: str(declarationType), Payload: str(payload)}
	}
	var declarations []components.CustomDeclarationModel
	typed([]any{
		declaration("com.apple.configuration.screensharing.host.settings", `{"PortOverride":5901}`),
		declaration("com.apple.configuration.diskmanagement.settings", `{"Restrictions":{"ExternalStorage":"ReadOnly"}}`),
		declaration("com.apple.configuration.diskmanagement.settings", `{"Restrictions":{"ExternalStorage":"ReadOnly"}}`),
		declaration("com.apple.configuration.diskmanagement.settings", `{"Restrictions":{"ExternalStorage":"Disallowed"}}`),
	}, &declarations)

	return ComponentBlockModel{
		Name:               str("Ordered"),
		SafariBookmarks:    &components.SafariBookmarksComponent{ManagedBookmarks: groups},
		SafariExtensions:   &components.SafariExtensionsComponent{ManagedExtensions: []components.ManagedExtensionModel{{ExtensionID: str("com.example.ext (ABCDE12345)"), State: str("Allowed"), AllowedDomains: allowed, DeniedDomains: denied}}},
		CustomDeclarations: &components.CustomDeclarationsComponent{Declarations: declarations},
		ServiceBackgroundTasks: &components.ServiceBackgroundTasksComponent{
			BackgroundTasks: tasks,
		},
		ServiceConfigurationFiles: &components.ServiceConfigurationFilesComponent{ServiceConfigFiles: files},
		SoftwareUpdateSettings:    &components.SoftwareUpdateSettingsComponent{BetaProgramEnrollment: str("AlwaysOn"), BetaOfferPrograms: programs},
	}
}

// wireStep renders a block the way the provider writes it, which is also how the blueprints
// service returns it: every probed component is stored verbatim.
func wireStep(t *testing.T, block ComponentBlockModel) blueprints.BlueprintStep {
	t.Helper()
	r := &BlueprintResource{}
	comps, diags := r.collectBlockComponents(context.Background(), block)
	if diags.HasError() {
		t.Fatalf("collecting components: %v", diags)
	}
	return blueprints.BlueprintStep{Name: new(block.Name.ValueString()), Components: comps}
}

// readBack maps a wire step into state the way Read does, with prior as the prior state's block,
// and stores the result through tfsdk.State.Set so a value a set could not hold fails the test.
func readBack(t *testing.T, step blueprints.BlueprintStep, prior ComponentBlockModel) ComponentBlockModel {
	t.Helper()
	ctx := context.Background()

	model := &BlueprintResourceModel{ComponentBlocks: []ComponentBlockModel{prior}}
	detail := &blueprints.BlueprintDetail{
		ID:              "bp-ordered",
		Name:            "ordered",
		Created:         time.Unix(0, 0),
		Updated:         time.Unix(0, 0),
		DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
		Scope:           &blueprints.BlueprintScope{DeviceGroups: []string{"00000000-0000-0000-0000-000000000001"}},
		Steps:           []blueprints.BlueprintStep{step},
	}
	if diags := updateModelFromAPIResponse(ctx, model, detail); diags.HasError() || diags.WarningsCount() > 0 {
		t.Fatalf("mapping the response raised diagnostics: %v", diags)
	}

	var resp resource.SchemaResponse
	NewBlueprintResource().Schema(ctx, resource.SchemaRequest{}, &resp)
	state := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatalf("storing the mapped state failed: %v", diags)
	}

	var stored BlueprintResourceModel
	if diags := state.Get(ctx, &stored); diags.HasError() {
		t.Fatalf("reading the stored state failed: %v", diags)
	}
	if len(stored.ComponentBlocks) != 1 {
		t.Fatalf("stored %d component blocks, want 1", len(stored.ComponentBlocks))
	}
	return stored.ComponentBlocks[0]
}

// TestOrderedCollections_RepeatsAndOrderRoundTrip reads back a step holding an identical repeat
// and a secondary-field variant in every ordered collection, written in two orders, and asserts
// every entry survives in the order the service returned it, with no diagnostic. Re-rendering the
// stored block must reproduce the wire step exactly, since any dropped, merged or reordered entry
// would change it.
func TestOrderedCollections_RepeatsAndOrderRoundTrip(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		block := orderedFixtureBlock(reversed)
		step := wireStep(t, block)
		stored := readBack(t, step, block)

		again := wireStep(t, stored)
		want, _ := json.Marshal(step.Components)
		got, _ := json.Marshal(again.Components)
		if string(want) != string(got) {
			t.Errorf("reversed=%v: state does not reproduce the stored step\nwant %s\ngot  %s", reversed, want, got)
		}

		if n := len(stored.SafariBookmarks.ManagedBookmarks); n != 4 {
			t.Errorf("reversed=%v: managed_bookmarks holds %d groups, want 4", reversed, n)
		}
		if n := len(stored.CustomDeclarations.Declarations); n != 4 {
			t.Errorf("reversed=%v: declaration holds %d entries, want 4", reversed, n)
		}
		if n := len(stored.SoftwareUpdateSettings.BetaOfferPrograms); n != 4 {
			t.Errorf("reversed=%v: beta_offer_programs holds %d entries, want 4", reversed, n)
		}
	}

	forward := readBack(t, wireStep(t, orderedFixtureBlock(false)), orderedFixtureBlock(false))
	backward := readBack(t, wireStep(t, orderedFixtureBlock(true)), orderedFixtureBlock(true))
	if first, last := forward.SoftwareUpdateSettings.BetaOfferPrograms[0], backward.SoftwareUpdateSettings.BetaOfferPrograms[3]; first != last {
		t.Errorf("a reorder was not kept: forward[0] = %+v, reversed[3] = %+v", first, last)
	}
	if first, last := forward.SafariBookmarks.ManagedBookmarks[0].GroupIdentifier, backward.SafariBookmarks.ManagedBookmarks[3].GroupIdentifier; first != last {
		t.Errorf("a bookmark group reorder was not kept: forward[0] = %s, reversed[3] = %s", first, last)
	}
}

// TestCustomDeclarations_PayloadKeyFollowsConfigurationOrder pins payloadKey to the 1-based
// position in the configuration, so a reorder re-keys only by moving, and a repeat gets its own
// key.
func TestCustomDeclarations_PayloadKeyFollowsConfigurationOrder(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		block := orderedFixtureBlock(reversed)
		raw, err := block.CustomDeclarations.ToRawConfiguration()
		if err != nil {
			t.Fatal(err)
		}
		var wire blueprints.CustomDeclarationsConfiguration
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if len(wire.Declarations) != len(block.CustomDeclarations.Declarations) {
			t.Fatalf("wrote %d declarations, want %d", len(wire.Declarations), len(block.CustomDeclarations.Declarations))
		}
		for i, declaration := range wire.Declarations {
			if declaration.PayloadKey != i+1 {
				t.Errorf("reversed=%v: declaration %d has payloadKey %d, want %d", reversed, i, declaration.PayloadKey, i+1)
			}
			if declaration.Type != block.CustomDeclarations.Declarations[i].Type.ValueString() {
				t.Errorf("reversed=%v: declaration %d is %s, want the configured %s", reversed, i, declaration.Type, block.CustomDeclarations.Declarations[i].Type.ValueString())
			}
		}
	}
}

// orderedCollectionPaths names every collection v6 made a list, as a path of attribute names
// from a component block. A name is entered through a single nested attribute or through a
// collection's element.
var orderedCollectionPaths = [][]string{
	{"custom_declarations", "declaration"},
	{"safari_bookmarks", "managed_bookmarks"},
	{"safari_bookmarks", "managed_bookmarks", "bookmarks"},
	{"safari_bookmarks", "managed_bookmarks", "bookmarks", "folder"},
	{"safari_extensions", "managed_extensions", "allowed_domains"},
	{"safari_extensions", "managed_extensions", "denied_domains"},
	{"service_background_tasks", "background_tasks"},
	{"service_background_tasks", "background_tasks", "launchd_configurations"},
	{"service_configuration_files", "service_config_files"},
	{"software_update_settings", "beta_offer_programs"},
}

// attributeAt walks a component block's attributes along names.
func attributeAt(t *testing.T, attributes map[string]schema.Attribute, names []string) schema.Attribute {
	t.Helper()
	var current schema.Attribute
	for i, name := range names {
		var ok bool
		current, ok = attributes[name]
		if !ok {
			t.Fatalf("%v: no attribute %q", names, name)
		}
		if i == len(names)-1 {
			break
		}
		switch a := current.(type) {
		case schema.SingleNestedAttribute:
			attributes = a.Attributes
		case schema.ListNestedAttribute:
			attributes = a.NestedObject.Attributes
		case schema.SetNestedAttribute:
			attributes = a.NestedObject.Attributes
		default:
			t.Fatalf("%v: cannot descend into %T", names, current)
		}
	}
	return current
}

// TestOrderedCollections_SchemaTypes asserts every converted collection is a list with no plan
// modifier, that the two genuine sets stay sets, and that the v5 prior schema still describes
// them as the sets v5 state was written with.
func TestOrderedCollections_SchemaTypes(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	NewBlueprintResource().Schema(ctx, resource.SchemaRequest{}, &resp)
	blocks := resp.Schema.Attributes["component_blocks"].(schema.ListNestedAttribute)
	live := blocks.NestedObject.Attributes

	prior := blueprintSchemaV5(ctx).Attributes["component_blocks"].(schema.ListNestedAttribute).NestedObject.Attributes

	for _, names := range orderedCollectionPaths {
		list, ok := attributeAt(t, live, names).(schema.ListNestedAttribute)
		if !ok {
			t.Errorf("%v is %T, want a ListNestedAttribute", names, attributeAt(t, live, names))
			continue
		}
		if len(list.PlanModifiers) > 0 || len(list.NestedObject.PlanModifiers) > 0 {
			t.Errorf("%v carries a plan modifier; a list stores the order written and needs none", names)
		}
		if _, ok := attributeAt(t, prior, names).(schema.SetNestedAttribute); !ok {
			t.Errorf("v5 prior %v is %T, want the SetNestedAttribute v5 wrote", names, attributeAt(t, prior, names))
		}
	}

	if _, ok := live["raw_component"].(schema.SetNestedAttribute); !ok {
		t.Error("raw_component must stay a set: the service does not keep the order of components sharing an identifier")
	}
	if _, ok := attributeAt(t, live, []string{"safari_extensions", "managed_extensions"}).(schema.SetNestedAttribute); !ok {
		t.Error("managed_extensions must stay a set: the wire keys it by extension identifier")
	}
	if _, ok := resp.Schema.Attributes["device_groups"].(schema.SetAttribute); !ok {
		t.Error("device_groups must stay a set of ids")
	}
}

// blueprintStateV5 is a frozen v5 document whose set-typed collections hold entries out of any
// sorted order, as Terraform writes a set.
const blueprintStateV5 = `{
  "id": "bp-v5",
  "name": "Frozen v5 blueprint",
  "description": null,
  "deployed": false,
  "device_groups": ["00000000-0000-0000-0000-000000000001"],
  "created": "2026-01-01T00:00:00Z",
  "updated": "2026-01-01T00:00:00Z",
  "deployment_state": "NOT_DEPLOYED",
  "timeouts": null,
  "component_blocks": [{
    "name": "Step 1",
    "activation_conditions": null,
    "apple_declarations": null,
    "legacy_payloads": null,
    "ai_governance": null,
    "audio_accessory_settings": null,
    "disk_management_settings": null,
    "math_settings": null,
    "passcode_policy": null,
    "safari_settings": null,
    "software_update": null,
    "raw_component": [
      {"identifier": "com.example.custom.z", "configuration": {"enabled": "true"}},
      {"identifier": "com.example.custom.a", "configuration": {"enabled": "false"}}
    ],
    "custom_declarations": {"declaration": [
      {"channel": "SYSTEM", "kind": "CONFIGURATION", "type": "com.apple.configuration.screensharing.host.settings", "payload": "{\"PortOverride\":5901}"},
      {"channel": "SYSTEM", "kind": "CONFIGURATION", "type": "com.apple.configuration.diskmanagement.settings", "payload": "{\"Restrictions\":{\"ExternalStorage\":\"ReadOnly\"}}"}
    ]},
    "safari_bookmarks": {"managed_bookmarks": [
      {"group_identifier": "g1", "title": "Work", "bookmarks": [
        {"type": "bookmark", "title": "Z", "url": "https://z.example.com", "folder": null},
        {"type": "folder", "title": "F", "url": null, "folder": [
          {"title": "m", "url": "https://m.example.com"},
          {"title": "b", "url": "https://b.example.com"}
        ]}
      ]},
      {"group_identifier": "g0", "title": "Home", "bookmarks": [
        {"type": "bookmark", "title": "Y", "url": "https://y.example.com", "folder": null}
      ]}
    ]},
    "safari_extensions": {"managed_extensions": [
      {"extension_id": "com.example.ext (ABCDE12345)", "state": "Allowed", "private_browsing": null,
       "allowed_domains": [{"domain": "z.example.com"}, {"domain": "a.example.com"}],
       "denied_domains": [{"domain": "d.example.com"}, {"domain": "c.example.com"}]}
    ]},
    "service_background_tasks": {"background_tasks": [
      {"task_type": "com.example.z", "task_description": "Z",
       "executable_asset_reference": {"data_url": "https://e.example.com/z.zip", "hash_sha_256": null, "content_type": "application/zip"},
       "launchd_configurations": [
         {"context": "daemon", "file_asset_reference": {"data_url": "https://e.example.com/2.plist", "hash_sha_256": null, "content_type": null}},
         {"context": "agent", "file_asset_reference": {"data_url": "https://e.example.com/1.plist", "hash_sha_256": null, "content_type": null}}
       ]},
      {"task_type": "com.example.a", "task_description": null, "executable_asset_reference": null, "launchd_configurations": null}
    ]},
    "service_configuration_files": {"service_config_files": [
      {"service_type": "com.apple.sshd", "data_asset_reference": null},
      {"service_type": "com.apple.bash", "data_asset_reference": null}
    ]},
    "software_update_settings": {
      "allow_standard_user_os_updates": null, "automatic_download": null, "automatic_install_os_updates": null,
      "automatic_install_security_updates": null, "beta_program_enrollment": "AlwaysOn",
      "beta_offer_programs": [{"token": "tok-z", "description": "Zed"}, {"token": "tok-a", "description": "Ay"}],
      "beta_require_program_token": null, "beta_require_program_description": null,
      "deferral_combined_period_days": null, "deferral_major_period_days": null, "deferral_minor_period_days": null,
      "deferral_system_period_days": null, "notifications_enabled": null, "rapid_security_response_enabled": null,
      "rapid_security_response_rollback_enabled": null, "recommended_cadence": null
    }
  }]
}`

// TestBlueprintStateV5UpgradesToLists runs the frozen v5 document through the v5 upgrader with the
// framework's decoding options and asserts every collection arrives as a list in the order v5 state
// held it, with nothing dropped.
func TestBlueprintStateV5UpgradesToLists(t *testing.T) {
	ctx := context.Background()
	r := NewBlueprintResource().(*BlueprintResource)

	upgrader, ok := r.UpgradeState(ctx)[5]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatal("no state upgrader with a prior schema is registered for schema version 5")
	}

	raw := tfprotov6.RawState{JSON: []byte(blueprintStateV5)}
	priorValue, err := raw.UnmarshalWithOpts(upgrader.PriorSchema.Type().TerraformType(ctx), tfprotov6.UnmarshalOpts{
		ValueFromJSONOpts: tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true},
	})
	if err != nil {
		t.Fatalf("the v5 prior schema does not decode v5 state: %v", err)
	}

	var current resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &current)
	req := resource.UpgradeStateRequest{State: &tfsdk.State{Raw: priorValue, Schema: *upgrader.PriorSchema}, RawState: &raw}
	resp := resource.UpgradeStateResponse{State: tfsdk.State{
		Raw:    tftypes.NewValue(current.Schema.Type().TerraformType(ctx), nil),
		Schema: current.Schema,
	}}
	upgrader.StateUpgrader(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrading v5 state failed: %v", resp.Diagnostics.Errors())
	}

	var upgraded BlueprintResourceModel
	if diags := resp.State.Get(ctx, &upgraded); diags.HasError() {
		t.Fatalf("reading the upgraded state failed: %v", diags.Errors())
	}
	if upgraded.Name.ValueString() != "Frozen v5 blueprint" || len(upgraded.ComponentBlocks) != 1 {
		t.Fatalf("the blueprint did not carry across: %+v", upgraded)
	}
	block := upgraded.ComponentBlocks[0]

	checks := []struct {
		name string
		got  []string
		want []string
	}{
		{"declaration", collect(block.CustomDeclarations.Declarations, func(d components.CustomDeclarationModel) string { return d.Type.ValueString() }), []string{"com.apple.configuration.screensharing.host.settings", "com.apple.configuration.diskmanagement.settings"}},
		{"managed_bookmarks", collect(block.SafariBookmarks.ManagedBookmarks, func(g components.BookmarkGroupModel) string { return g.GroupIdentifier.ValueString() }), []string{"g1", "g0"}},
		{"bookmarks", collect(block.SafariBookmarks.ManagedBookmarks[0].Bookmarks, func(b components.BookmarkModel) string { return b.Title.ValueString() }), []string{"Z", "F"}},
		{"folder", collect(block.SafariBookmarks.ManagedBookmarks[0].Bookmarks[1].Folder, func(b components.UrlBookmarkModel) string { return b.Title.ValueString() }), []string{"m", "b"}},
		{"allowed_domains", collect(block.SafariExtensions.ManagedExtensions[0].AllowedDomains, func(d components.ManagedExtensionDomainModel) string { return d.Domain.ValueString() }), []string{"z.example.com", "a.example.com"}},
		{"denied_domains", collect(block.SafariExtensions.ManagedExtensions[0].DeniedDomains, func(d components.ManagedExtensionDomainModel) string { return d.Domain.ValueString() }), []string{"d.example.com", "c.example.com"}},
		{"background_tasks", collect(block.ServiceBackgroundTasks.BackgroundTasks, func(b components.ServiceBackgroundTaskModel) string { return b.TaskType.ValueString() }), []string{"com.example.z", "com.example.a"}},
		{"launchd_configurations", collect(block.ServiceBackgroundTasks.BackgroundTasks[0].LaunchdConfigurations, func(l components.LaunchdItemModel) string { return l.Context.ValueString() }), []string{"daemon", "agent"}},
		{"service_config_files", collect(block.ServiceConfigurationFiles.ServiceConfigFiles, func(f components.ServiceConfigFileModel) string { return f.ServiceType.ValueString() }), []string{"com.apple.sshd", "com.apple.bash"}},
		{"beta_offer_programs", collect(block.SoftwareUpdateSettings.BetaOfferPrograms, func(p components.BetaProgramModel) string { return p.Token.ValueString() }), []string{"tok-z", "tok-a"}},
	}
	for _, check := range checks {
		if !slices.Equal(check.got, check.want) {
			t.Errorf("%s = %v, want %v in the order v5 state held it", check.name, check.got, check.want)
		}
	}

	if _, ok := resp.State.Raw.Type().(tftypes.Object).AttributeTypes["component_blocks"].(tftypes.List).ElementType.(tftypes.Object).AttributeTypes["software_update_settings"].(tftypes.Object).AttributeTypes["beta_offer_programs"].(tftypes.List); !ok {
		t.Error("upgraded state does not hold beta_offer_programs as a list")
	}
}

func collect[T any](items []T, key func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, key(item))
	}
	return out
}

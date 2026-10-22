// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*SetWallpaperAction)(nil)
var _ action.ActionWithConfigure = (*SetWallpaperAction)(nil)
var _ action.ActionWithConfigValidators = (*SetWallpaperAction)(nil)

// positiveInteger matches an icon id: the Icon resource reports its id as a
// string, so the attribute accepts that string unchanged.
var positiveInteger = regexp.MustCompile(`^[1-9][0-9]*$`)

// wallpaperCommand is the command name the Classic mobile device command
// endpoint takes: the plain name, not the prose-annotated one the spec lists.
const wallpaperCommand = "Wallpaper"

// SetWallpaperAction sets the wallpaper on supervised mobile devices.
//
// Unlike the commands on POST /pro/v2/mdm/commands this one rides the Classic
// mobile device command endpoint, which is the only place the command exists.
// Four wire facts decide the shape, probed 2026-10-22 against simulated
// devices: the body's <mobile_device> entries are matched by <id>, <serial_number>
// or <udid> but NOT <management_id> (400 "Unable to match mobile_device", the
// same words as an unknown device), so serial numbers go straight through and
// Management IDs are resolved to serial numbers first; a body naming devices
// overrides the URL, so every device goes in the body; wallpaper_id is checked
// against the icons Jamf Pro holds, while wallpaper_content is not checked at
// all (a bad image is only discovered on the device), which is why the image is
// validated here at plan time; and the server folds a repeat Wallpaper command
// into the one already pending for the device.
type SetWallpaperAction struct {
	mdmAction
}

type SetWallpaperActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Location      types.String         `tfsdk:"location"`
	Image         types.String         `tfsdk:"image"`
	IconID        types.String         `tfsdk:"icon_id"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewSetWallpaperAction() action.Action {
	return &SetWallpaperAction{}
}

func (a *SetWallpaperAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_set_wallpaper"
}

func (a *SetWallpaperAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := selectorAttributes("mobile device",
		"",
		"Each is looked up to find its serial number before the command is sent, so `serial_numbers` avoids that lookup.")
	attrs["location"] = actionschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Where the wallpaper appears: `" + strings.Join(wallpaperLocationNames(), "`, `") + "`.",
		Validators:          []validator.String{stringvalidator.OneOf(wallpaperLocationNames()...)},
	}
	attrs["image"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The picture to use, as a base64-encoded PNG or JPEG, for example `filebase64(\"wallpaper.png\")`. Set this or `icon_id`.",
		Validators:          []validator.String{wallpaperImage{}},
	}
	attrs["icon_id"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "ID of an icon already in Jamf Pro to use as the picture, such as the `id` of a `jamfplatform_pro_icon`. Set this or `image`.",
		Validators: []validator.String{
			stringvalidator.RegexMatches(positiveInteger, "must be a positive whole number"),
		},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Sets the wallpaper on one or more supervised iPhones and iPads. Choose the picture with `image` or `icon_id`, and where it appears with `location`." +
			"\n\nIf a device already has this command waiting, for example because it is offline, Jamf Pro keeps that command and queues no other, so a changed picture or location does not reach the device." +
			batchNote + setWallpaperPrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *SetWallpaperAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return append(deviceTargetListConfigValidators(),
		actionvalidator.ExactlyOneOf(
			path.MatchRoot("image"),
			path.MatchRoot("icon_id"),
		),
	)
}

func (a *SetWallpaperAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *SetWallpaperAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) || !a.ensureClassicClient(resp) {
		return
	}

	var data SetWallpaperActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serials, ok := a.resolveWallpaperSerials(ctx, resp, data)
	if !ok {
		return
	}

	if len(serials) > batchWarnThreshold {
		resp.Diagnostics.AddWarning(
			"Large Command Batch",
			fmt.Sprintf("Set Wallpaper targets %d devices in a single request. Jamf Pro documents no maximum, but the request is all-or-nothing: if any one device cannot be matched the whole batch fails with an error that does not identify which. Consider splitting this into smaller invocations.", len(serials)),
		)
	}

	request, err := setWallpaperRequest(data, serials)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Wallpaper Configuration", helpers.APIErrorDetail(err))
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Requesting Set Wallpaper for %d device(s)", len(serials))})

	issued, err := a.classic.CreateMobileDeviceCommandByCommand(ctx, wallpaperCommand, request)
	if err != nil {
		resp.Diagnostics.AddError(
			"Set Wallpaper Failed",
			fmt.Sprintf("Unable to queue Set Wallpaper for %d device(s)%s: %s", len(serials), batchTargetSuffix(serials), helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Set Wallpaper accepted for %d device(s) (command %s)", len(serials), derefString(issued.UUID))})
}

// resolveWallpaperSerials returns the serial numbers of every targeted device:
// the listed serials as written, then the Management IDs resolved to theirs.
// A device selected both ways appears once, since a repeated <mobile_device>
// entry would only name the same device twice.
func (a *SetWallpaperAction) resolveWallpaperSerials(ctx context.Context, resp *action.InvokeResponse, data SetWallpaperActionModel) ([]string, bool) {
	var serials, ids []string

	if !data.SerialNumbers.IsNull() && !data.SerialNumbers.IsUnknown() {
		resp.Diagnostics.Append(data.SerialNumbers.ElementsAs(ctx, &serials, false)...)
	}
	if !data.ManagementIDs.IsNull() && !data.ManagementIDs.IsUnknown() {
		resp.Diagnostics.Append(data.ManagementIDs.ElementsAs(ctx, &ids, false)...)
	}
	if resp.Diagnostics.HasError() {
		return nil, false
	}

	if len(ids) > 0 {
		fromIDs, ok := a.resolveSerialsForManagementIDs(ctx, resp, ids)
		if !ok {
			return nil, false
		}
		serials = append(serials, fromIDs...)
	}

	if len(serials) == 0 {
		resp.Diagnostics.AddError(
			"Missing Device Identifier",
			"Specify at least one of management_ids or serial_numbers to select the devices.",
		)
		return nil, false
	}
	return uniqueStrings(serials), true
}

// derefString reads an optional response field for a progress message.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// uniqueStrings drops repeats and keeps first-seen order.
func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// setWallpaperRequest builds the Classic command body. Devices are named by
// serial number in the body rather than in the URL: a body naming devices
// overrides the URL, and a parameterised command with no devices in its body
// is refused even for a single device that would otherwise work.
func setWallpaperRequest(data SetWallpaperActionModel, serials []string) (*proclassic.MobileDeviceCommandPost, error) {
	setting, ok := wallpaperLocations[data.Location.ValueString()]
	if !ok {
		return nil, fmt.Errorf("location %q is not one of %s", data.Location.ValueString(), strings.Join(wallpaperLocationNames(), ", "))
	}

	command := wallpaperCommand
	general := &proclassic.MobileDeviceCommandPostGeneral{
		Command:          &command,
		WallpaperSetting: &setting,
	}
	if !data.Image.IsNull() {
		general.WallpaperContent = data.Image.ValueStringPointer()
	}
	if !data.IconID.IsNull() {
		id, err := strconv.Atoi(data.IconID.ValueString())
		if err != nil {
			return nil, fmt.Errorf("icon_id %q is not a whole number", data.IconID.ValueString())
		}
		general.WallpaperID = &id
	}

	devices := make([]proclassic.MobileDeviceCommandPostMobileDevicesMobileDeviceItem, 0, len(serials))
	for _, serial := range serials {
		devices = append(devices, proclassic.MobileDeviceCommandPostMobileDevicesMobileDeviceItem{SerialNumber: new(serial)})
	}

	return &proclassic.MobileDeviceCommandPost{
		General:       general,
		MobileDevices: &proclassic.MobileDeviceCommandPostMobileDevices{MobileDevice: &devices},
	}, nil
}

// wallpaperImage checks that `image` is base64 holding a PNG or JPEG.
//
// Jamf Pro checks none of it: a string that is not base64, base64 that is not an
// image, and a 5.7 MB PNG were all answered 201 (SDK v1.9.0 wire facts), and the
// failure only shows on the device. So this is the only place a bad image is
// caught before it is sent to every device in the batch. An unknown value, such
// as one built from another resource, is skipped until it is known.
type wallpaperImage struct{}

func (wallpaperImage) Description(context.Context) string {
	return "value must be a base64-encoded PNG or JPEG image"
}

func (v wallpaperImage) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (wallpaperImage) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Wallpaper Image",
			"The image is not valid base64. Encode the file with filebase64(), for example filebase64(\"wallpaper.png\").")
		return
	}
	switch http.DetectContentType(raw) {
	case "image/png", "image/jpeg":
	default:
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Wallpaper Image",
			"The decoded data is not a PNG or JPEG image. Jamf Pro would accept these bytes and the device would reject them.")
	}
}

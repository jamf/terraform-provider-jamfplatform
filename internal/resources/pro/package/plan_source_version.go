// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
)

// sourceStatTimeout bounds the metadata request ModifyPlan makes to the store.
// It is one small request, so a stall is a network problem and must not hold a
// plan for the length of an upload timeout.
const sourceStatTimeout = 60 * time.Second

// uploadDerivedAttributes are the attributes Jamf Pro derives from the uploaded
// bytes. A plan that expects the file to change leaves each of them Unknown.
var uploadDerivedAttributes = []string{"sha3_512", "sha256", "md5", "hash_value", "size"}

// markUploadDerivedUnknown leaves every attribute Jamf Pro derives from the
// uploaded bytes Unknown in the plan.
func markUploadDerivedUnknown(ctx context.Context, resp *resource.ModifyPlanResponse) {
	for _, name := range uploadDerivedAttributes {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(name), types.StringUnknown())...)
	}
}

// markerVerdict is what a plan does with package_file_source_version.
type markerVerdict int

const (
	// markerNone plans null: the source is not in cloud storage.
	markerNone markerVerdict = iota
	// markerKeep plans the value in state: the object is the one last read.
	markerKeep
	// markerReplace plans the object's current marker as a known value, with
	// the upload-derived attributes Unknown: the object is new or has changed.
	markerReplace
	// markerDefer plans Unknown, with the upload-derived attributes Unknown:
	// the object cannot be examined yet, so the apply decides.
	markerDefer
)

// markerFacts is what planMarkerVerdict decides from. stateMarker is empty when
// state holds none, and objectMarker is empty when the object was not examined
// or the store reported no marker.
type markerFacts struct {
	native        bool
	creating      bool
	sourceChanged bool
	stateMarker   string
	examined      bool
	objectMarker  string
}

// planMarkerVerdict decides the planned value of package_file_source_version.
//
// The marker in state is kept only when everything says the bytes in Jamf Pro
// are the bytes the object holds now: the resource exists, the source string
// is the one the marker was recorded for, state has a marker, and the store
// reports the same one. Anything less is a difference. That includes a null
// marker, which is what an adopted package and a package whose file was
// replaced in Jamf Pro both carry, and a changed source string whose new
// object happens to share the ETag, because two objects with the same content
// can.
//
// An object that was not examined defers to the apply, and so does one whose
// store reported no marker: without a marker there is nothing to pin the read
// to or to compare next time.
func planMarkerVerdict(f markerFacts) markerVerdict {
	switch {
	case !f.native:
		return markerNone
	case !f.examined || f.objectMarker == "":
		return markerDefer
	case !f.creating && !f.sourceChanged && f.stateMarker != "" && f.stateMarker == f.objectMarker:
		return markerKeep
	default:
		return markerReplace
	}
}

// planSourceVersion owns the planned value of package_file_source_version and
// decides, for an s3:// or azblob:// source, whether the plan expects the
// package file to change.
//
// No attribute plan modifier carries the value. UseStateForUnknown would copy
// a null state value forward, and for a package with a local source that reads
// as "(known after apply)" on every update; the value has to be decided from
// the source and the store, which only ModifyPlan can see together.
//
// For a cloud storage source it asks the store for the object's marker, one
// metadata request, and compares it with the one in state (planMarkerVerdict).
// When the object has changed, the new marker is planned as a known value. The
// apply then reads the object pinned to that marker, so the bytes uploaded are
// the ones this plan described, or the apply fails before uploading anything.
//
// An ETag that changed under an unchanged source string adds a warning, because
// the Unknown size and hashes read as a certain re-upload when the apply may
// find the bytes unchanged.
//
// Three cases make no request or cannot use the answer:
//
//   - A source carrying ?versionId= names one immutable version. Once state
//     holds its marker for the same source string, nothing can have changed.
//   - A file_sources value the source's store needs is unknown, because it
//     derives from a resource Terraform has not created. The plan defers with a
//     warning naming the value.
//   - The object does not exist. It may be created earlier in the same apply,
//     by a resource this one depends on, so the plan defers with a warning
//     in place of failing. A mistyped name surfaces during apply.
//
// Every other failure is an error on package_file_source. A credential that
// does not work is not something an apply would fix, and skipping the check
// silently, as the local-file check does for an unreadable file, would let a
// changed object go unnoticed.
//
// Terraform runs ModifyPlan again when it applies. An object replaced between
// a saved plan and its apply therefore changes the marker this function plans,
// and Terraform reports the provider's final plan as inconsistent. A provider
// cannot intercept that; running the plan again is the remedy.
func (r *PackageResource) planSourceVersion(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	marker := path.Root("package_file_source_version")

	var source types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("package_file_source"), &source)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if source.IsUnknown() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, marker, types.StringUnknown())...)
		return
	}

	facts := markerFacts{
		native:   isConfiguredString(source) && filesources.Native(source.ValueString()),
		creating: req.State.Raw.IsNull(),
	}
	if !facts.native {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, marker, types.StringNull())...)
		return
	}
	if !facts.creating {
		var stateSource, stateMarker types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("package_file_source"), &stateSource)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, marker, &stateMarker)...)
		if resp.Diagnostics.HasError() {
			return
		}
		facts.sourceChanged = !stateSource.Equal(source)
		facts.stateMarker = stateMarker.ValueString()
	}

	src := source.ValueString()
	switch {
	case filesources.Versioned(src) && !facts.creating && !facts.sourceChanged && facts.stateMarker != "":
		facts.examined, facts.objectMarker = true, facts.stateMarker
	case len(r.sources.UnresolvedFor(src)) > 0:
		resp.Diagnostics.AddAttributeWarning(path.Root("package_file_source"), "Package file not examined during this plan", fmt.Sprintf(
			"The provider's %s is not known until apply, so the object cannot be read yet. "+
				"The plan shows the package file as changing, and the apply decides whether to upload it.",
			strings.Join(r.sources.UnresolvedFor(src), ", ")))
	default:
		statCtx, cancel := context.WithTimeout(ctx, sourceStatTimeout)
		attributes, err := r.sources.Stat(statCtx, src)
		cancel()
		switch {
		case errors.Is(err, filesources.ErrNotFound):
			resp.Diagnostics.AddAttributeWarning(path.Root("package_file_source"), "Package file not found during this plan",
				"The object does not exist yet. If another resource in this configuration creates it, the apply reads it then. "+
					"Otherwise check the bucket and the object name: the apply fails if the object is still missing.")
		case err != nil:
			summary, detail := sourceProblem(err)
			resp.Diagnostics.AddAttributeError(path.Root("package_file_source"), summary, detail)
			return
		case attributes.Marker == "":
			resp.Diagnostics.AddAttributeError(path.Root("package_file_source"), "Cloud storage reports no ETag for the package file",
				"The provider tells a changed object from an unchanged one by its ETag, and reads it pinned to that ETag. "+
					"This store returned none, so every plan would show the file as changing. Use an http(s):// URL for this store instead.")
			return
		default:
			facts.examined, facts.objectMarker = true, attributes.Marker
		}
	}

	switch planMarkerVerdict(facts) {
	case markerKeep:
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, marker, types.StringValue(facts.stateMarker))...)
	case markerReplace:
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, marker, types.StringValue(facts.objectMarker))...)
		if !facts.creating {
			markUploadDerivedUnknown(ctx, resp)
		}
		if !facts.creating && !facts.sourceChanged && facts.stateMarker != "" {
			resp.Diagnostics.AddAttributeWarning(path.Root("package_file_source"), "Package file ETag changed",
				"Size and hashes show as known after apply. "+
					"The apply uploads the file only if its contents differ from what Jamf Pro holds.")
		}
	case markerDefer:
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, marker, types.StringUnknown())...)
		if !facts.creating {
			markUploadDerivedUnknown(ctx, resp)
		}
	}
}

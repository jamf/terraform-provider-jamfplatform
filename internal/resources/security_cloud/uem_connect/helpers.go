// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// reverseMapping inverts a value-translation table. Collisions cannot arise: the
// tables it is used on are bijections between this resource's vocabulary and the
// stored one, and mappings_test.go pins that.
func reverseMapping(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[v] = k
	}
	return out
}

// sortedMapKeys returns a map's keys in a stable order, so a validator's accepted
// set and the documentation rendered from it do not reshuffle between builds.
func sortedMapKeys(in map[string]string) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Machine-readable error codes Jamf Security Cloud returns on the UEM Connect
// endpoints. Wire-probed against production EU on 2026-08-28.
//
// Only NOT_ENTITLED is in the SDK's generated ApiErrorItemCode enum, and it is
// taken from there so a rename upstream breaks the build. The rest are string
// literals because that enum is declared by the DNS namespace's spec schema and
// the UEM Connect spec declares no equivalent — there is nothing generated to
// reference. mappings_test.go pins that the SDK still lacks them, so if a future
// spec adds them the literals stop being the only option and the test says so.
const (
	codeConfigAlreadyExists = "CONNECTOR_CONFIG_ALREADY_EXISTS"
	codeConnectionFailed    = "UEM_CONNECTION_FAILED"
	codeConnectorDisabled   = "CONNECTOR_DISABLED"
	codeValidationFailed    = "VALIDATION_FAILED"
	codeNotFound            = "NOT_FOUND"

	codeNotEntitled = securitycloud.ApiErrorItemCodeNotEntitled
)

// groupIDFormatMessage is the fragment Jamf Security Cloud puts in the
// VALIDATION_FAILED description when a group identifier is malformed. Matched on
// so a generic validation failure can be attributed to the attribute that caused
// it, since the error body names no field.
const groupIDFormatMessage = "must start with 'mobile_' or 'computer_'"

// appendCreateDiagnostics turns a create failure into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// Three of these are worth translating because the cause is not what the message
// describes. CONNECTOR_CONFIG_ALREADY_EXISTS blames "an incompatible UEM vendor",
// but it fires for an identical vendor too — there is simply one connector per
// tenant, and the fix is to import the existing one rather than to change vendor.
// UEM_CONNECTION_FAILED covers a wrong address, an unreachable instance and bad
// credentials with one code and no indication which. VALIDATION_FAILED arrives
// with a null field, so the group identifier case has to be recognised from its
// description before it can be attached to an attribute.
func appendCreateDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch {
		case detail.Code == codeConfigAlreadyExists:
			diags.AddError(
				"A UEM Connect integration already exists for this tenant",
				"Jamf Security Cloud holds one UEM Connect integration per tenant, so this one cannot be created "+
					"alongside the existing one. Import the existing integration into Terraform instead of "+
					"creating a new one, or remove it in Jamf Security Cloud first. The reported reason names an "+
					"incompatible vendor, but the limit applies whatever vendor the existing integration uses. "+
					"Reported by Jamf Security Cloud: "+detail.Description,
			)
		case detail.Code == codeConnectionFailed:
			diags.AddError(
				"Jamf Security Cloud could not reach the Jamf Pro instance",
				"The connection test run while creating the integration failed. Jamf Security Cloud reports one "+
					"reason for three different causes, so check all of them: the Jamf Pro address is wrong or "+
					"not a full URL including its scheme; the instance is not reachable from Jamf Security "+
					"Cloud's published addresses; or the supplied credentials are not valid on that instance. "+
					"Reported by Jamf Security Cloud: "+detail.Description,
			)
		case detail.Code == codeValidationFailed && strings.Contains(detail.Description, groupIDFormatMessage):
			diags.AddAttributeError(
				path.Root("group_membership_mapping").AtName("mappings"),
				"Invalid Jamf Pro group identifier",
				"A group mapping names a Jamf Pro group in a form Jamf Security Cloud does not accept. For a Jamf "+
					"Pro integration the identifier is `computer_` or `mobile_` followed by the group's number, "+
					"for example `computer_12`. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case detail.Code == codeValidationFailed:
			diags.AddError(
				"Jamf Security Cloud rejected the UEM Connect integration",
				"Reported by Jamf Security Cloud: "+detail.Description,
			)
		case detail.Code == codeNotEntitled:
			diags.AddError(
				"Tenant not entitled to Jamf Security Cloud UEM Connect",
				"The credentials authenticated successfully but this tenant does not have the UEM Connect surface "+
					"enabled. Contact Jamf to have it provisioned. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// appendUpdateDiagnostics translates an update failure. It shares the group
// identifier and entitlement cases with create and adds nothing of its own:
// neither the one-per-tenant limit nor the connection test applies once the
// integration exists, since the settings endpoints do not re-test connectivity.
func appendUpdateDiagnostics(diags *diag.Diagnostics, err error) bool {
	return appendCreateDiagnostics(diags, err)
}

// isNotFound reports whether an error is Jamf Security Cloud saying the
// integration is gone, which Read treats as a removal rather than a failure.
//
// Two 404s that no Jamf service produced are excluded, because Read acts on this
// by deleting the resource from state and each of them reports that the request
// reached no Jamf service: the gateway's own unrouted reply
// (helpers.IsGatewayUnrouted) and an edge error page, which CloudFront serves
// with a 404 among other statuses (helpers.IsEdgeBlocked). Either one read as
// absence empties a live UEM Connect integration out of state on the next
// refresh, and Delete then reports success for an integration it never touched.
func isNotFound(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	if helpers.IsGatewayUnrouted(err) {
		return false
	}
	if helpers.IsEdgeBlocked(err) {
		return false
	}
	if apiErr.HasStatus(http.StatusNotFound) {
		return true
	}
	for _, detail := range apiErr.Details() {
		if detail.Code == codeNotFound {
			return true
		}
	}
	return false
}

// timePointerValue renders an optional timestamp in RFC 3339, the form every other
// timestamp in this provider takes.
func timePointerValue(v *time.Time) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(v.Format(time.RFC3339))
}

// appendMissingIntegrationDiagnostics refuses a read of a tenant that holds no UEM
// Connect integration.
//
// This is the guard in front of `page.Results[0]`, so a regression here is a
// provider panic rather than a diagnostic — and unlike most of this package's
// guards it is reachable in entirely ordinary use, by reading the data source
// before creating an integration. It lives here rather than inline in the data
// source so a unit test can drive it: the shape it guards is a plain SDK struct,
// while the data source's Read needs a live client.
func appendMissingIntegrationDiagnostics(page *securitycloud.ConnectorPage) diag.Diagnostics {
	var diags diag.Diagnostics
	if page != nil && len(page.Results) > 0 {
		return diags
	}
	diags.AddError(
		"No UEM Connect integration on this tenant",
		"Jamf Security Cloud reports no UEM Connect integration for this tenant. Set one up before reading it, "+
			"with jamfplatform_security_cloud_uem_connect or in the Jamf Security Cloud admin UI.",
	)
	return diags
}

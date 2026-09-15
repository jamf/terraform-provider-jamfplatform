// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Machine-readable error codes Jamf Security Cloud returns on the custom hostname
// mappings endpoint. Wire-probed against production EU on 2026-08-29. All three are
// in the SDK's generated ApiErrorItemCode enum, which the DNS namespace declares in
// its spec schema, so all three are taken from there rather than restated as
// literals — checked individually rather than reasoned about as a set, per
// STYLE_GUIDE §Enum values and error codes come from the SDK.
const (
	codeInvalidField     = securitycloud.ApiErrorItemCodeInvalidField
	codeListSizeExceeded = securitycloud.ApiErrorItemCodeListSizeExceeded
	codeNotEntitled      = securitycloud.ApiErrorItemCodeNotEntitled
)

// appendWriteDiagnostics turns a write failure into the most specific diagnostic the
// error body supports, and reports whether it recognised one.
//
// Every code here should have been prevented at plan time, so reaching apply means
// the provider's rules and the server's have diverged. Naming the attribute anyway is
// what makes that diagnosable: the raw bodies are a bare "Invalid field value." with
// a null field, and a size violation that sometimes names an indexed field and
// sometimes names nothing.
//
// The one failure this cannot help with is a duplicate host name across two mappings,
// which the endpoint answers with a 500 carrying an empty errors array. There is no
// code to match and nothing in the body to quote, which is precisely why the
// uniqueness check is a plan-time validator rather than a translated diagnostic.
//
// Every diagnostic here carries err.Error() as well as the detail's own description,
// matching appendDuplicateHostnameHint below. The description alone drops the status,
// the method, the URL and the trace identifier the error carries, which is exactly
// what a support conversation needs — and the two helpers reporting the same failure
// in different amounts of detail is a difference with no reason behind it.
//
// A detail whose code this does not recognise gets a diagnostic quoting the code
// verbatim rather than being skipped. Skipping it was silent in the worst way: the
// caller falls back to a generic message only when nothing matched, so an
// unrecognised detail arriving alongside a recognised one vanished with no diagnostic
// and no log line.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeInvalidField:
			diags.AddAttributeError(
				path.Root("mappings"),
				"Hostname mapping not accepted",
				"Jamf Security Cloud refuses one of these mappings. Check that each host name is a valid name "+
					"with no wildcard, that every `ipv4_addresses` entry is an IPv4 address and every "+
					"`ipv6_addresses` entry an IPv6 address, and that each mapping sets at least one of the two. "+
					"Reported by Jamf Security Cloud: "+detail.Description+". Underlying error: "+helpers.APIErrorDetail(err),
			)
		case codeListSizeExceeded:
			diags.AddAttributeError(
				path.Root("mappings"),
				"Hostname mapping collection size out of range",
				"A collection here is outside the size Jamf Security Cloud accepts: `mappings` takes 1 to 500 "+
					"entries, and each mapping takes at most 10 `ipv4_addresses` and 10 `ipv6_addresses`. "+
					"Reported by Jamf Security Cloud: "+detail.Description+". Underlying error: "+helpers.APIErrorDetail(err),
			)
		case codeNotEntitled:
			diags.AddError(
				"Tenant not entitled to Jamf Security Cloud custom DNS",
				"The credentials authenticated successfully but this tenant does not have the custom DNS surface "+
					"enabled. Contact Jamf to have it provisioned. Reported by Jamf Security Cloud: "+
					detail.Description+". Underlying error: "+helpers.APIErrorDetail(err),
			)
		default:
			diags.AddError(
				"Jamf Security Cloud rejected the write",
				"Unrecognised error code "+detail.Code+": "+detail.Description+
					". Underlying error: "+helpers.APIErrorDetail(err),
			)
		}
		matched = true
	}
	return matched
}

// appendDuplicateHostnameHint adds guidance when a write fails with a bare 500 and no
// error details.
//
// A duplicate host name across two mappings is the one cause wire-probing found for
// that response, and the plan-time uniqueness validator should have caught it — so
// reaching here means either the validator missed a case or the endpoint has a second
// way of failing this way. Saying both is more use than surfacing the bare status.
func appendDuplicateHostnameHint(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(http.StatusInternalServerError) || len(apiErr.Details()) > 0 {
		return false
	}
	diags.AddAttributeError(
		path.Root("mappings"),
		"Jamf Security Cloud rejected the hostname mappings without saying why",
		"The write failed with an internal server error carrying no detail. The known cause is the same host "+
			"name appearing in more than one mapping, which this provider normally catches before the write — so "+
			"check for a repeated `hostname`, and if there is none, this is worth reporting to Jamf. Underlying "+
			"error: "+helpers.APIErrorDetail(err),
	)
	return true
}

// storedMappingCount returns the number of mappings the tenant currently holds,
// treating a nil response as none.
func storedMappingCount(existing *securitycloud.MappingList) int {
	if existing == nil {
		return 0
	}
	return len(existing.Results)
}

// storedMappingsMatchPlan reports whether the mappings already on the tenant are
// exactly the set the plan proposes to write.
//
// Adopting the provider's own interrupted write is not a clobber, and separating the
// two needs a comparison against the planned value rather than a test for presence.
// Create makes three calls under one timeout — a preflight read, the write, and a
// confirming read whose result becomes state — and the third can fail after the second
// has landed: the create timeout expiring, a transient 5xx, a dropped connection.
// Create then returns an error without setting state, so the tenant is configured and
// Terraform holds nothing. The next apply re-enters Create and meets the provider's
// own work. A presence-only preflight refuses that byte-identical retry exactly as it
// refuses a real clobber, and tells the operator to import mappings nobody authored
// elsewhere.
//
// The plan side goes through buildMappingsWriteInput, which is the same
// canonicalisation the write itself applies — a null address list becomes an empty
// array, an unset boolean becomes false — so the two sides cannot differ over
// something the wire does not carry. Comparison is order-insensitive on the mapping
// collection and on each address list, because the server reorders both. Anything
// short of equality leaves the refusal in place: a tenant holding one extra mapping is
// a tenant whose mappings this configuration did not write.
func storedMappingsMatchPlan(ctx context.Context, planned types.Set, existing *securitycloud.MappingList) (bool, diag.Diagnostics) {
	input, diags := buildMappingsWriteInput(ctx, planned)
	if diags.HasError() {
		return false, diags
	}
	if len(input) != storedMappingCount(existing) {
		return false, diags
	}
	return slices.Equal(sortedMappingKeys(input), sortedMappingKeys(existing.Results)), diags
}

// sortedMappingKeys renders each mapping as a canonical string and sorts the result,
// so two collections holding the same mappings in different orders compare equal.
func sortedMappingKeys(mappings []securitycloud.Mapping) []string {
	keys := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		keys = append(keys, mappingKey(mapping))
	}
	slices.Sort(keys)
	return keys
}

// mappingKey renders one mapping as a canonical string. The separators are the ASCII
// record and unit separators, which no host name or address can contain, so no value
// can be mistaken for a field boundary.
func mappingKey(mapping securitycloud.Mapping) string {
	return strings.Join([]string{
		mapping.Hostname,
		strings.Join(sortedAddresses(mapping.ARecords), "\x1f"),
		strings.Join(sortedAddresses(mapping.AaaaRecords), "\x1f"),
		strconv.FormatBool(boolOrFalse(mapping.SecureDns)),
		strconv.FormatBool(boolOrFalse(mapping.Ztna)),
	}, "\x1e")
}

// sortedAddresses copies an optional address list into a sorted slice, treating nil
// and empty alike — absent and empty are the same thing for an address list here.
func sortedAddresses(addresses *[]string) []string {
	if addresses == nil {
		return nil
	}
	out := slices.Clone(*addresses)
	slices.Sort(out)
	return out
}

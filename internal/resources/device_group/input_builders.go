// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"cmp"
	"slices"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// expandDeviceGroupCriteria converts Terraform models into API representations.
func expandDeviceGroupCriteria(criteria []DeviceGroupCriteriaModel) []devicegroups.DeviceGroupCriteriaRepresentationV1 {
	if len(criteria) == 0 {
		return nil
	}
	result := make([]devicegroups.DeviceGroupCriteriaRepresentationV1, 0, len(criteria))
	for idx, c := range criteria {
		if !helpers.IsConfiguredValue(c.AttributeName) {
			continue
		}

		operator := ""
		if helpers.IsConfiguredValue(c.Operator) {
			operator = strings.ToUpper(c.Operator.ValueString())
		}

		attributeValue := ""
		if helpers.IsConfiguredValue(c.AttributeValue) {
			attributeValue = c.AttributeValue.ValueString()
		}

		joinType := ""
		if helpers.IsConfiguredValue(c.JoinType) {
			joinType = strings.ToUpper(c.JoinType.ValueString())
		}

		crit := devicegroups.DeviceGroupCriteriaRepresentationV1{
			AttributeName:         c.AttributeName.ValueString(),
			Operator:              operator,
			AttributeValue:        attributeValue,
			JoinType:              joinType,
			HasOpeningParenthesis: c.HasOpeningParenthesis.ValueBoolPointer(),
			HasClosingParenthesis: c.HasClosingParenthesis.ValueBoolPointer(),
		}
		if helpers.IsConfiguredValue(c.Order) {
			crit.Order = int(c.Order.ValueInt64())
		} else {
			crit.Order = idx
		}
		result = append(result, crit)
	}
	slices.SortStableFunc(result, func(a, b devicegroups.DeviceGroupCriteriaRepresentationV1) int {
		return cmp.Compare(a.Order, b.Order)
	})
	return result
}

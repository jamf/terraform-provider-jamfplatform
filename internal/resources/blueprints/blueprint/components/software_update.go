// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var (
	// timeHHMMRegex validates HH:mm 24-hour time format.
	timeHHMMRegex = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
	// semverRegex validates semantic version strings (major.minor or major.minor.patch).
	semverRegex = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	// dateTimeRegex validates RFC3339 date-time format without timezone.
	dateTimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`)
)

// SoftwareUpdateComponent represents a strongly-typed software update enforcement component.
type SoftwareUpdateComponent struct {
	EnforcementType     types.String `tfsdk:"enforcement_type"`
	DeploymentTime      types.String `tfsdk:"deployment_time"`
	EnforceAfterDays    types.Int64  `tfsdk:"enforce_after_days"`
	IgnoreMajorVersions types.Bool   `tfsdk:"ignore_major_versions"`
	TargetOSVersion     types.String `tfsdk:"target_os_version"`
	TargetLocalDateTime types.String `tfsdk:"target_local_date_time"`
	DetailsURLValue     types.String `tfsdk:"details_url_value"`
}

// SoftwareUpdateComponentSchema returns the Terraform schema for software update component.
func SoftwareUpdateComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enforcement_type": schema.StringAttribute{
			MarkdownDescription: "Type of enforcement. Automatically set to `AUTOMATIC` when `deployment_time` or `enforce_after_days` is specified, or `MANUAL` when `target_os_version` or `target_local_date_time` is specified.",
			Computed:            true,
		},
		"deployment_time": schema.StringAttribute{
			MarkdownDescription: "For automatic enforcement. Local device time to install the update. Format: `HH:mm` (24-hour). Cannot be used with `target_os_version` or `target_local_date_time`.",
			Optional:            true,
			Validators: []validator.String{
				stringvalidator.RegexMatches(
					timeHHMMRegex,
					"Value must be in HH:mm format (e.g., 14:30)",
				),
				stringvalidator.AlsoRequires(
					path.MatchRelative().AtParent().AtName("enforce_after_days"),
					path.MatchRelative().AtParent().AtName("ignore_major_versions"),
				),
				stringvalidator.ConflictsWith(
					path.MatchRelative().AtParent().AtName("target_os_version"),
					path.MatchRelative().AtParent().AtName("target_local_date_time"),
				),
			},
		},
		"ignore_major_versions": schema.BoolAttribute{
			MarkdownDescription: "Whether to ignore major OS versions when enforcing updates. Only applicable for automatic enforcement. Cannot be used with `target_os_version` or `target_local_date_time`.",
			Optional:            true,
			Validators: []validator.Bool{
				boolvalidator.AlsoRequires(
					path.MatchRelative().AtParent().AtName("deployment_time"),
					path.MatchRelative().AtParent().AtName("enforce_after_days"),
				),
				boolvalidator.ConflictsWith(
					path.MatchRelative().AtParent().AtName("target_os_version"),
					path.MatchRelative().AtParent().AtName("target_local_date_time"),
				),
			},
		},
		"enforce_after_days": schema.Int64Attribute{
			MarkdownDescription: "For automatic enforcement. Days after release to enforce the update. Maximum is `30`. Cannot be used with `target_os_version` or `target_local_date_time`.",
			Optional:            true,
			Validators: []validator.Int64{
				int64validator.Between(0, 30),
				int64validator.AlsoRequires(
					path.MatchRelative().AtParent().AtName("deployment_time"),
					path.MatchRelative().AtParent().AtName("ignore_major_versions"),
				),
				int64validator.ConflictsWith(
					path.MatchRelative().AtParent().AtName("target_os_version"),
					path.MatchRelative().AtParent().AtName("target_local_date_time"),
				),
			},
		},
		"target_os_version": schema.StringAttribute{
			MarkdownDescription: "For manual enforcement. Target OS version. Format: `major.minor[.patch]`. Cannot be used with `deployment_time`, `enforce_after_days`, or `ignore_major_versions`.",
			Optional:            true,
			Validators: []validator.String{
				stringvalidator.RegexMatches(
					semverRegex,
					"Value must be a valid semantic version (e.g., 10.15.7)",
				),
				stringvalidator.AlsoRequires(
					path.MatchRelative().AtParent().AtName("target_local_date_time"),
				),
				stringvalidator.ConflictsWith(
					path.MatchRelative().AtParent().AtName("deployment_time"),
					path.MatchRelative().AtParent().AtName("enforce_after_days"),
					path.MatchRelative().AtParent().AtName("ignore_major_versions"),
				),
			},
		},
		"target_local_date_time": schema.StringAttribute{
			MarkdownDescription: "For manual enforcement. Local device date and time to enforce the software update. Format: RFC3339 date-time. Cannot be used with `deployment_time`, `enforce_after_days`, or `ignore_major_versions`.",
			Optional:            true,
			Validators: []validator.String{
				stringvalidator.RegexMatches(
					dateTimeRegex,
					"Value must be a valid RFC3339 date-time (e.g., 2023-10-05T14:48:00)",
				),
				stringvalidator.AlsoRequires(
					path.MatchRelative().AtParent().AtName("target_os_version"),
				),
				stringvalidator.ConflictsWith(
					path.MatchRelative().AtParent().AtName("deployment_time"),
					path.MatchRelative().AtParent().AtName("enforce_after_days"),
					path.MatchRelative().AtParent().AtName("ignore_major_versions"),
				),
			},
		},
		"details_url_value": schema.StringAttribute{
			MarkdownDescription: "URL of a web page with the details about the software update.",
			Optional:            true,
		},
	}
}

// ToRawConfiguration converts the strongly-typed component to raw JSON configuration.
func (c *SoftwareUpdateComponent) ToRawConfiguration() (json.RawMessage, error) {
	config := make(map[string]any)

	isAutomatic := helpers.IsConfiguredValue(c.DeploymentTime) || helpers.IsConfiguredValue(c.EnforceAfterDays)
	isManual := helpers.IsConfiguredValue(c.TargetOSVersion) || helpers.IsConfiguredValue(c.TargetLocalDateTime)
	ignoreMajor := helpers.IsConfiguredValue(c.IgnoreMajorVersions) && c.IgnoreMajorVersions.ValueBool()

	if isAutomatic {
		config["enforcementType"] = blueprints.SwUpdateConfigurationEnforcementTypeAutomatic

		if ignoreMajor {
			config["strategy"] = blueprints.SwUpdateAutomaticConfigurationStrategySemantic

			minorRule := make(map[string]any)
			if helpers.IsConfiguredValue(c.DeploymentTime) {
				minorRule["deploymentTime"] = c.DeploymentTime.ValueString()
			}
			if helpers.IsConfiguredValue(c.EnforceAfterDays) {
				minorRule["enforceAfterDays"] = c.EnforceAfterDays.ValueInt64()
			}

			if len(minorRule) > 0 {
				config["rules"] = map[string]any{
					"minor": minorRule,
				}
			}
		} else {
			config["strategy"] = blueprints.SwUpdateAutomaticConfigurationStrategyLatest

			if helpers.IsConfiguredValue(c.DeploymentTime) {
				config["deploymentTime"] = c.DeploymentTime.ValueString()
			}
			if helpers.IsConfiguredValue(c.EnforceAfterDays) {
				config["enforceAfterDays"] = c.EnforceAfterDays.ValueInt64()
			}
		}
	} else if isManual {
		config["enforcementType"] = blueprints.SwUpdateConfigurationEnforcementTypeManual
	}

	if helpers.IsConfiguredValue(c.TargetOSVersion) {
		config["targetOSVersion"] = c.TargetOSVersion.ValueString()
	}
	if helpers.IsConfiguredValue(c.TargetLocalDateTime) {
		config["targetLocalDateTime"] = c.TargetLocalDateTime.ValueString()
	}

	detailsURL := setStringField(c.DetailsURLValue, "")
	if helpers.IsConfiguredValue(c.DetailsURLValue) && c.DetailsURLValue.ValueString() == "" {
		detailsURL["Included"] = false
	}
	config["detailsURL"] = detailsURL

	return json.Marshal(config)
}

// FromRawConfiguration populates the strongly-typed component from raw JSON configuration.
func (c *SoftwareUpdateComponent) FromRawConfiguration(raw json.RawMessage) error {
	var rawConfig map[string]any
	if err := json.Unmarshal(raw, &rawConfig); err != nil {
		return err
	}

	c.IgnoreMajorVersions = types.BoolNull()

	if enforcementType, exists := rawConfig["enforcementType"]; exists {
		if enforcementTypeStr, ok := enforcementType.(string); ok {
			c.EnforcementType = types.StringValue(enforcementTypeStr)
		}
	}

	strategy := ""
	if strategyValue, exists := rawConfig["strategy"]; exists {
		if strategyStr, ok := strategyValue.(string); ok {
			strategy = strategyStr
			switch strategyStr {
			case blueprints.SwUpdateAutomaticConfigurationStrategySemantic:
				c.IgnoreMajorVersions = types.BoolValue(true)
			case blueprints.SwUpdateAutomaticConfigurationStrategyLatest:
				c.IgnoreMajorVersions = types.BoolValue(false)
			}
		}
	}

	automaticFieldsDetected := false
	useSemanticRules := strategy == blueprints.SwUpdateAutomaticConfigurationStrategySemantic

	if useSemanticRules {
		if rulesValue, exists := rawConfig["rules"]; exists {
			if rulesMap, ok := rulesValue.(map[string]any); ok {
				if minorValue, exists := rulesMap["minor"]; exists {
					if minorMap, ok := minorValue.(map[string]any); ok {
						if deploymentTime, exists := minorMap["deploymentTime"]; exists {
							if deploymentTimeStr, ok := deploymentTime.(string); ok {
								c.DeploymentTime = types.StringValue(deploymentTimeStr)
								automaticFieldsDetected = true
							}
						}
						if enforceAfterDays, exists := minorMap["enforceAfterDays"]; exists {
							if enforceAfterDaysFloat, ok := enforceAfterDays.(float64); ok {
								c.EnforceAfterDays = types.Int64Value(int64(enforceAfterDaysFloat))
								automaticFieldsDetected = true
							}
						}
					}
				}
			}
		}
	} else {
		if deploymentTime, exists := rawConfig["deploymentTime"]; exists {
			if deploymentTimeStr, ok := deploymentTime.(string); ok {
				c.DeploymentTime = types.StringValue(deploymentTimeStr)
				automaticFieldsDetected = true
			}
		}
		if enforceAfterDays, exists := rawConfig["enforceAfterDays"]; exists {
			if enforceAfterDaysFloat, ok := enforceAfterDays.(float64); ok {
				c.EnforceAfterDays = types.Int64Value(int64(enforceAfterDaysFloat))
				automaticFieldsDetected = true
			}
		}
	}

	if targetOSVersion, exists := rawConfig["targetOSVersion"]; exists {
		if targetOSVersionStr, ok := targetOSVersion.(string); ok {
			c.TargetOSVersion = types.StringValue(targetOSVersionStr)
		}
	}
	if targetLocalDateTime, exists := rawConfig["targetLocalDateTime"]; exists {
		if targetLocalDateTimeStr, ok := targetLocalDateTime.(string); ok {
			c.TargetLocalDateTime = types.StringValue(targetLocalDateTimeStr)
		}
	}

	if detailsURL, exists := rawConfig["detailsURL"]; exists {
		if detailsURLMap, ok := detailsURL.(map[string]any); ok {
			if value, exists := detailsURLMap["Value"]; exists {
				if valueStr, ok := value.(string); ok && valueStr != "" {
					c.DetailsURLValue = types.StringValue(valueStr)
				}
			}
		}
	}

	if automaticFieldsDetected && !helpers.IsConfiguredValue(c.IgnoreMajorVersions) {
		c.IgnoreMajorVersions = types.BoolValue(false)
	}

	if !helpers.IsConfiguredValue(c.EnforcementType) {
		if helpers.IsConfiguredValue(c.TargetOSVersion) || helpers.IsConfiguredValue(c.TargetLocalDateTime) {
			c.EnforcementType = types.StringValue(blueprints.SwUpdateConfigurationEnforcementTypeManual)
		} else if automaticFieldsDetected {
			c.EnforcementType = types.StringValue(blueprints.SwUpdateConfigurationEnforcementTypeAutomatic)
		}
	}

	return nil
}

// GetIdentifier returns the component identifier for software update.
func (c *SoftwareUpdateComponent) GetIdentifier() string {
	return "com.jamf.ddm.sw-updates"
}

// ToClientComponent converts the strongly-typed component to a blueprints.Component.
func (c *SoftwareUpdateComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}

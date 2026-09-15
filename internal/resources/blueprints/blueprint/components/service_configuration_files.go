// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ServiceConfigurationFilesComponent represents a strongly-typed service configuration files component.
type ServiceConfigurationFilesComponent struct {
	ServiceConfigFiles []ServiceConfigFileModel `tfsdk:"service_config_files"`
}

// ServiceConfigDataAssetRefModel represents a data asset reference for service configuration files.
type ServiceConfigDataAssetRefModel struct {
	DataURL     types.String `tfsdk:"data_url"`
	HashSHA256  types.String `tfsdk:"hash_sha_256"`
	ContentType types.String `tfsdk:"content_type"`
}

// ServiceConfigFileModel represents a service configuration file.
type ServiceConfigFileModel struct {
	ServiceType        types.String                    `tfsdk:"service_type"`
	DataAssetReference *ServiceConfigDataAssetRefModel `tfsdk:"data_asset_reference"`
}

// ServiceConfigurationFilesComponentSchema returns the Terraform schema for service configuration files component.
func ServiceConfigurationFilesComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"service_config_files": schema.SetNestedAttribute{
			MarkdownDescription: "Set of service configuration files.",
			Optional:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"service_type": schema.StringAttribute{
						MarkdownDescription: "The identifier of the system service with managed configuration files.",
						Required:            true,
					},
					"data_asset_reference": schema.SingleNestedAttribute{
						MarkdownDescription: "Reference to the configuration data asset.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"data_url": schema.StringAttribute{
								MarkdownDescription: "URL that hosts the configuration data.",
								Required:            true,
							},
							"hash_sha_256": schema.StringAttribute{
								MarkdownDescription: "SHA-256 hash of the data.",
								Optional:            true,
							},
							"content_type": schema.StringAttribute{
								MarkdownDescription: "Media type of the data. Always `application/zip` for service configuration files.",
								Computed:            true,
							},
						},
					},
				},
			},
		},
	}
}

// buildServiceConfigDataAssetRef converts a ServiceConfigDataAssetRefModel to a map for API serialization.
func buildServiceConfigDataAssetRef(ref *ServiceConfigDataAssetRefModel) map[string]any {
	inner := map[string]any{
		"DataURL":     ref.DataURL.ValueString(),
		"ContentType": "application/zip",
	}
	if helpers.IsConfiguredValue(ref.HashSHA256) {
		inner["Hash-SHA-256"] = ref.HashSHA256.ValueString()
	}
	return map[string]any{"Reference": inner}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *ServiceConfigurationFilesComponent) ToRawConfiguration() (json.RawMessage, error) {
	files := make([]map[string]any, 0, len(c.ServiceConfigFiles))

	for _, cf := range c.ServiceConfigFiles {
		f := map[string]any{
			"ServiceType": cf.ServiceType.ValueString(),
		}
		if cf.DataAssetReference != nil {
			f["DataAssetReference"] = buildServiceConfigDataAssetRef(cf.DataAssetReference)
		}
		files = append(files, f)
	}

	return json.Marshal(map[string]any{"serviceConfigFiles": files})
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *ServiceConfigurationFilesComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	filesRaw, _ := cfg["serviceConfigFiles"].([]any)
	files := make([]ServiceConfigFileModel, 0, len(filesRaw))
	for _, fRaw := range filesRaw {
		fMap, ok := fRaw.(map[string]any)
		if !ok {
			continue
		}
		cf := ServiceConfigFileModel{}
		if v, ok := fMap["ServiceType"].(string); ok {
			cf.ServiceType = types.StringValue(v)
		}
		if refMap, ok := fMap["DataAssetReference"].(map[string]any); ok {
			innerMap, ok := refMap["Reference"].(map[string]any)
			if ok {
				dataRef := &ServiceConfigDataAssetRefModel{}
				if v, ok := innerMap["DataURL"].(string); ok {
					dataRef.DataURL = types.StringValue(v)
				}
				if v, ok := innerMap["Hash-SHA-256"].(string); ok {
					dataRef.HashSHA256 = types.StringValue(v)
				}
				ct := "application/zip"
				if v, ok := innerMap["ContentType"].(string); ok {
					ct = v
				}
				dataRef.ContentType = types.StringValue(ct)
				cf.DataAssetReference = dataRef
			}
		}
		files = append(files, cf)
	}

	c.ServiceConfigFiles = files
	return nil
}

// GetIdentifier returns the component identifier for service configuration files.
func (c *ServiceConfigurationFilesComponent) GetIdentifier() string {
	return "com.jamf.ddm.service-configuration-files"
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *ServiceConfigurationFilesComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}

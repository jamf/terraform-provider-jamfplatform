// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ServiceBackgroundTasksComponent represents a strongly-typed service background tasks component.
type ServiceBackgroundTasksComponent struct {
	BackgroundTasks []ServiceBackgroundTaskModel `tfsdk:"background_tasks"`
}

// ServiceBackgroundTaskModel represents a background task configuration.
type ServiceBackgroundTaskModel struct {
	TaskType                 types.String       `tfsdk:"task_type"`
	TaskDescription          types.String       `tfsdk:"task_description"`
	ExecutableAssetReference *DataAssetRefModel `tfsdk:"executable_asset_reference"`
	LaunchdConfigurations    []LaunchdItemModel `tfsdk:"launchd_configurations"`
}

// DataAssetRefModel represents a data asset reference.
type DataAssetRefModel struct {
	DataURL     types.String `tfsdk:"data_url"`
	HashSHA256  types.String `tfsdk:"hash_sha_256"`
	ContentType types.String `tfsdk:"content_type"`
}

// LaunchdItemModel represents a launchd configuration item.
type LaunchdItemModel struct {
	Context            types.String       `tfsdk:"context"`
	FileAssetReference *DataAssetRefModel `tfsdk:"file_asset_reference"`
}

// ServiceBackgroundTasksComponentSchema returns the Terraform schema for service background tasks component.
func ServiceBackgroundTasksComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"background_tasks": schema.SetNestedAttribute{
			MarkdownDescription: "Set of background tasks.",
			Optional:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"task_type": schema.StringAttribute{
						MarkdownDescription: "Task type identifier.",
						Required:            true,
					},
					"task_description": schema.StringAttribute{
						MarkdownDescription: "Task description.",
						Optional:            true,
					},
					"executable_asset_reference": schema.SingleNestedAttribute{
						MarkdownDescription: "Reference to the executable asset.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"data_url": schema.StringAttribute{
								MarkdownDescription: "URL that hosts the executable data.",
								Required:            true,
							},
							"hash_sha_256": schema.StringAttribute{
								MarkdownDescription: "SHA-256 hash of the data.",
								Optional:            true,
							},
							"content_type": schema.StringAttribute{
								MarkdownDescription: "Media type of the data. Always `application/zip` for executable assets.",
								Computed:            true,
							},
						},
					},
					"launchd_configurations": schema.SetNestedAttribute{
						MarkdownDescription: "Launchd configuration items.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"context": schema.StringAttribute{
									MarkdownDescription: "Launchd context. Valid values are `daemon`, `agent`.",
									Required:            true,
									Validators:          []validator.String{stringvalidator.OneOf("daemon", "agent")},
								},
								"file_asset_reference": schema.SingleNestedAttribute{
									MarkdownDescription: "Reference to the configuration file asset.",
									Required:            true,
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
											MarkdownDescription: "Media type of the data.",
											Optional:            true,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// buildDataAssetReference converts a DataAssetRefModel to a map for API serialization.
func buildDataAssetReference(ref *DataAssetRefModel, defaultContentType string) map[string]any {
	inner := map[string]any{
		"DataURL": ref.DataURL.ValueString(),
	}
	ct := defaultContentType
	if helpers.IsConfiguredValue(ref.ContentType) {
		ct = ref.ContentType.ValueString()
	}
	if ct != "" {
		inner["ContentType"] = ct
	}
	if helpers.IsConfiguredValue(ref.HashSHA256) {
		inner["Hash-SHA-256"] = ref.HashSHA256.ValueString()
	}
	return map[string]any{"Reference": inner}
}

// dataAssetRefFromMap converts a raw map to a DataAssetRefModel.
func dataAssetRefFromMap(refMap map[string]any, defaultContentType string) *DataAssetRefModel {
	innerMap, ok := refMap["Reference"].(map[string]any)
	if !ok {
		return nil
	}
	m := &DataAssetRefModel{}
	if v, ok := innerMap["DataURL"].(string); ok {
		m.DataURL = types.StringValue(v)
	}
	if v, ok := innerMap["Hash-SHA-256"].(string); ok {
		m.HashSHA256 = types.StringValue(v)
	}
	ct := defaultContentType
	if v, ok := innerMap["ContentType"].(string); ok {
		ct = v
	}
	m.ContentType = types.StringValue(ct)
	return m
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *ServiceBackgroundTasksComponent) ToRawConfiguration() (json.RawMessage, error) {
	tasks := make([]map[string]any, 0, len(c.BackgroundTasks))

	for _, task := range c.BackgroundTasks {
		t := map[string]any{
			"TaskType": task.TaskType.ValueString(),
		}
		if helpers.IsConfiguredValue(task.TaskDescription) {
			t["TaskDescription"] = task.TaskDescription.ValueString()
		}
		if task.ExecutableAssetReference != nil {
			t["ExecutableAssetReference"] = buildDataAssetReference(task.ExecutableAssetReference, "application/zip")
		}
		if len(task.LaunchdConfigurations) > 0 {
			launchdItems := make([]map[string]any, 0, len(task.LaunchdConfigurations))
			for _, lc := range task.LaunchdConfigurations {
				item := map[string]any{
					"Context": lc.Context.ValueString(),
				}
				if lc.FileAssetReference != nil {
					item["FileAssetReference"] = buildDataAssetReference(lc.FileAssetReference, "")
				}
				launchdItems = append(launchdItems, item)
			}
			t["LaunchdConfigurations"] = launchdItems
		}
		tasks = append(tasks, t)
	}

	return json.Marshal(map[string]any{"backgroundTasks": tasks})
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *ServiceBackgroundTasksComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	tasksRaw, _ := cfg["backgroundTasks"].([]any)
	tasks := make([]ServiceBackgroundTaskModel, 0, len(tasksRaw))
	for _, taskRaw := range tasksRaw {
		taskMap, ok := taskRaw.(map[string]any)
		if !ok {
			continue
		}
		task := ServiceBackgroundTaskModel{}
		if v, ok := taskMap["TaskType"].(string); ok {
			task.TaskType = types.StringValue(v)
		}
		if v, ok := taskMap["TaskDescription"].(string); ok {
			task.TaskDescription = types.StringValue(v)
		}
		if refMap, ok := taskMap["ExecutableAssetReference"].(map[string]any); ok {
			task.ExecutableAssetReference = dataAssetRefFromMap(refMap, "application/zip")
		}
		if launchdRaw, ok := taskMap["LaunchdConfigurations"].([]any); ok {
			launchdConfigs := make([]LaunchdItemModel, 0, len(launchdRaw))
			for _, lcRaw := range launchdRaw {
				lcMap, ok := lcRaw.(map[string]any)
				if !ok {
					continue
				}
				item := LaunchdItemModel{}
				if v, ok := lcMap["Context"].(string); ok {
					item.Context = types.StringValue(v)
				}
				if fileRefMap, ok := lcMap["FileAssetReference"].(map[string]any); ok {
					item.FileAssetReference = dataAssetRefFromMap(fileRefMap, "")
				}
				launchdConfigs = append(launchdConfigs, item)
			}
			task.LaunchdConfigurations = launchdConfigs
		}
		tasks = append(tasks, task)
	}

	c.BackgroundTasks = tasks
	return nil
}

// GetIdentifier returns the component identifier for service background tasks.
func (c *ServiceBackgroundTasksComponent) GetIdentifier() string {
	return "com.jamf.ddm.service-background-tasks"
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *ServiceBackgroundTasksComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}

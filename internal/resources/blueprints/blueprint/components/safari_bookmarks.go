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

// SafariBookmarksComponent represents a strongly-typed Safari bookmarks component.
type SafariBookmarksComponent struct {
	ManagedBookmarks []BookmarkGroupModel `tfsdk:"managed_bookmarks"`
}

// BookmarkGroupModel represents a group of managed bookmarks.
type BookmarkGroupModel struct {
	GroupIdentifier types.String    `tfsdk:"group_identifier"`
	Title           types.String    `tfsdk:"title"`
	Bookmarks       []BookmarkModel `tfsdk:"bookmarks"`
}

// BookmarkModel represents a bookmark item.
type BookmarkModel struct {
	Type   types.String       `tfsdk:"type"`
	Title  types.String       `tfsdk:"title"`
	URL    types.String       `tfsdk:"url"`
	Folder []UrlBookmarkModel `tfsdk:"folder"`
}

// UrlBookmarkModel represents a URL bookmark.
type UrlBookmarkModel struct {
	Title types.String `tfsdk:"title"`
	URL   types.String `tfsdk:"url"`
}

// GetIdentifier returns the component identifier for Safari bookmarks.
func (c *SafariBookmarksComponent) GetIdentifier() string {
	return "com.jamf.ddm.safari-bookmarks"
}

// SafariBookmarksComponentSchema returns the Terraform schema for Safari bookmarks component.
func SafariBookmarksComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"managed_bookmarks": schema.SetNestedAttribute{
			MarkdownDescription: "Set of managed bookmark groups.",
			Optional:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"group_identifier": schema.StringAttribute{
						MarkdownDescription: "Unique identifier for this group of managed bookmarks.",
						Required:            true,
					},
					"title": schema.StringAttribute{
						MarkdownDescription: "The name of the bookmarks folder.",
						Required:            true,
					},
					"bookmarks": schema.SetNestedAttribute{
						MarkdownDescription: "Set of bookmarks in this group.",
						Required:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"type": schema.StringAttribute{
									MarkdownDescription: "Type of bookmark. Valid values are `bookmark` (URL bookmark) or `folder` (bookmark folder).",
									Optional:            true,
									Validators:          []validator.String{stringvalidator.OneOf("bookmark", "folder")},
								},
								"title": schema.StringAttribute{
									MarkdownDescription: "The title of the folder shown in Safari.",
									Required:            true,
								},
								"url": schema.StringAttribute{
									MarkdownDescription: "The URL for a direct bookmark. Folders do not take one.",
									Optional:            true,
								},
								"folder": schema.SetNestedAttribute{
									MarkdownDescription: "Bookmarks within this folder.",
									Optional:            true,
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"title": schema.StringAttribute{
												MarkdownDescription: "The title of the bookmark shown in Safari.",
												Required:            true,
											},
											"url": schema.StringAttribute{
												MarkdownDescription: "The URL for the bookmark item.",
												Required:            true,
											},
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

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *SafariBookmarksComponent) ToRawConfiguration() (json.RawMessage, error) {
	if len(c.ManagedBookmarks) == 0 {
		return json.Marshal(blueprints.SafariBookmarksConfiguration{ManagedBookmarks: []blueprints.BookmarkGroup{}})
	}

	groups := make([]blueprints.BookmarkGroup, 0, len(c.ManagedBookmarks))
	for _, group := range c.ManagedBookmarks {
		g := blueprints.BookmarkGroup{}

		if helpers.IsConfiguredValue(group.GroupIdentifier) {
			g.GroupIdentifier = group.GroupIdentifier.ValueString()
		}
		if helpers.IsConfiguredValue(group.Title) {
			g.Title = group.Title.ValueString()
		}

		bookmarks := make([]blueprints.BookmarkItem, 0, len(group.Bookmarks))
		for _, bookmark := range group.Bookmarks {
			typeValue := bookmark.Type.ValueString()
			switch typeValue {
			case "bookmark", "url", "":
				item := blueprints.URLBookmarkItem{
					Type:  blueprints.BookmarkItemTypeBookmark,
					Title: bookmark.Title.ValueString(),
					URL:   bookmark.URL.ValueString(),
				}
				bookmarks = append(bookmarks, blueprints.BookmarkItem{Type: blueprints.BookmarkItemTypeBookmark, BOOKMARK: &item})
			case "folder":
				folder := make([]blueprints.URLBookmarkItem, 0, len(bookmark.Folder))
				for _, ub := range bookmark.Folder {
					folder = append(folder, blueprints.URLBookmarkItem{
						Type:  blueprints.BookmarkItemTypeBookmark,
						Title: ub.Title.ValueString(),
						URL:   ub.URL.ValueString(),
					})
				}
				item := blueprints.FolderBookmarkItem{
					Type:   blueprints.BookmarkItemTypeFolder,
					Title:  bookmark.Title.ValueString(),
					Folder: &folder,
				}
				bookmarks = append(bookmarks, blueprints.BookmarkItem{Type: blueprints.BookmarkItemTypeFolder, FOLDER: &item})
			}
		}
		g.Bookmarks = bookmarks
		groups = append(groups, g)
	}

	cfg := blueprints.SafariBookmarksConfiguration{ManagedBookmarks: groups}
	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *SafariBookmarksComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg blueprints.SafariBookmarksConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	managedBookmarks := make([]BookmarkGroupModel, 0, len(cfg.ManagedBookmarks))
	for _, g := range cfg.ManagedBookmarks {
		group := BookmarkGroupModel{
			GroupIdentifier: types.StringValue(g.GroupIdentifier),
			Title:           types.StringValue(g.Title),
		}

		bookmarks := make([]BookmarkModel, 0, len(g.Bookmarks))
		for _, bRaw := range g.Bookmarks {
			bm := BookmarkModel{}
			switch bRaw.Type {
			case blueprints.BookmarkItemTypeBookmark:
				bm.Type = types.StringValue("bookmark")
				if bRaw.BOOKMARK != nil {
					bm.Title = types.StringValue(bRaw.BOOKMARK.Title)
					bm.URL = types.StringValue(bRaw.BOOKMARK.URL)
				}
			case blueprints.BookmarkItemTypeFolder:
				bm.Type = types.StringValue("folder")
				if bRaw.FOLDER != nil {
					bm.Title = types.StringValue(bRaw.FOLDER.Title)
					if bRaw.FOLDER.Folder != nil {
						folder := make([]UrlBookmarkModel, 0, len(*bRaw.FOLDER.Folder))
						for _, ub := range *bRaw.FOLDER.Folder {
							folder = append(folder, UrlBookmarkModel{
								Title: types.StringValue(ub.Title),
								URL:   types.StringValue(ub.URL),
							})
						}
						bm.Folder = folder
					}
				}
			default:
				bm.Type = types.StringValue(bRaw.Type)
			}
			bookmarks = append(bookmarks, bm)
		}
		group.Bookmarks = bookmarks
		managedBookmarks = append(managedBookmarks, group)
	}

	c.ManagedBookmarks = managedBookmarks
	return nil
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *SafariBookmarksComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}

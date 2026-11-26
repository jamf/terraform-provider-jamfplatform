// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// headerNamePattern is an HTTP field name: one or more token characters.
var headerNamePattern = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")

// fileSourcesModel is the decoded file_sources block. Every level is held as a
// framework value, never a Go struct pointer, because any of it can be unknown
// while Terraform plans and a typed pointer cannot carry unknown.
type fileSourcesModel struct {
	S3    types.Object `tfsdk:"s3"`
	Azure types.Object `tfsdk:"azure"`
	HTTPS types.List   `tfsdk:"https"`
}

// fileSourcesS3Model is the decoded file_sources.s3 block.
type fileSourcesS3Model struct {
	Region                    types.String `tfsdk:"region"`
	Profile                   types.String `tfsdk:"profile"`
	SharedConfigFiles         types.List   `tfsdk:"shared_config_files"`
	SharedCredentialsFiles    types.List   `tfsdk:"shared_credentials_files"`
	AccessKey                 types.String `tfsdk:"access_key"`
	SecretKey                 types.String `tfsdk:"secret_key"`
	Token                     types.String `tfsdk:"token"`
	AssumeRole                types.Object `tfsdk:"assume_role"`
	AssumeRoleWithWebIdentity types.Object `tfsdk:"assume_role_with_web_identity"`
	Endpoints                 types.Object `tfsdk:"endpoints"`
	UsePathStyle              types.Bool   `tfsdk:"use_path_style"`
}

// fileSourcesS3AssumeRoleModel is the decoded file_sources.s3.assume_role block.
type fileSourcesS3AssumeRoleModel struct {
	RoleARN     types.String `tfsdk:"role_arn"`
	SessionName types.String `tfsdk:"session_name"`
	ExternalID  types.String `tfsdk:"external_id"`
}

// fileSourcesS3WebIdentityModel is the decoded
// file_sources.s3.assume_role_with_web_identity block.
type fileSourcesS3WebIdentityModel struct {
	RoleARN              types.String `tfsdk:"role_arn"`
	SessionName          types.String `tfsdk:"session_name"`
	WebIdentityToken     types.String `tfsdk:"web_identity_token"`
	WebIdentityTokenFile types.String `tfsdk:"web_identity_token_file"`
}

// fileSourcesS3EndpointsModel is the decoded file_sources.s3.endpoints block.
type fileSourcesS3EndpointsModel struct {
	S3  types.String `tfsdk:"s3"`
	STS types.String `tfsdk:"sts"`
}

// fileSourcesAzureModel is the decoded file_sources.azure block.
type fileSourcesAzureModel struct {
	TenantID             types.String `tfsdk:"tenant_id"`
	ClientID             types.String `tfsdk:"client_id"`
	UseOIDC              types.Bool   `tfsdk:"use_oidc"`
	OIDCToken            types.String `tfsdk:"oidc_token"`
	OIDCTokenFilePath    types.String `tfsdk:"oidc_token_file_path"`
	OIDCRequestURL       types.String `tfsdk:"oidc_request_url"`
	OIDCRequestToken     types.String `tfsdk:"oidc_request_token"`
	UseMSI               types.Bool   `tfsdk:"use_msi"`
	MSIEndpoint          types.String `tfsdk:"msi_endpoint"`
	UseCLI               types.Bool   `tfsdk:"use_cli"`
	ClientSecret         types.String `tfsdk:"client_secret"`
	ClientSecretFilePath types.String `tfsdk:"client_secret_file_path"`
	Environment          types.String `tfsdk:"environment"`
}

// fileSourcesHTTPSModel is one decoded file_sources.https entry.
type fileSourcesHTTPSModel struct {
	Host        types.String `tfsdk:"host"`
	BearerToken types.String `tfsdk:"bearer_token"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	Header      types.Object `tfsdk:"header"`
}

// fileSourcesHTTPSHeaderModel is the decoded header block of an https entry.
type fileSourcesHTTPSHeaderModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

// sibling addresses an attribute beside the one being validated, inside the
// same nested object.
func sibling(name string) path.Expression {
	return path.MatchRelative().AtParent().AtName(name)
}

// fileSourcesAttribute is the provider's file_sources block.
//
// The s3 and azure blocks take the argument names of Terraform's s3 and
// azurerm state backends, so a configuration that already authenticates a
// backend can be copied across. Only the methods this provider supports are
// present: the role-assumption extras, the skip flags, a custom CA bundle and
// the Azure storage-key methods are not, and each can be added without
// breaking a configuration written today.
//
// Every secret is Sensitive and nothing more. Provider configuration is never
// written to state, so the WriteOnly rule for resource secrets does not apply;
// it is carried in a saved plan file, which the description says.
//
// The validators cover only what is decidable from the block alone. Each store
// also reads its own environment variables, so a rule such as "client_id is
// required with use_oidc" cannot be checked here: the client ID may come from
// ARM_CLIENT_ID. Those rules are checked on the resolved values, where the
// credential is built. The one pairing held to configuration is the S3 static
// key: an access key written here with its secret left to the environment is
// refused, because a key pair split across two places is a mistake far more
// often than an intention.
func fileSourcesAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		MarkdownDescription: "Credentials and endpoints for reading a package file from Amazon S3, Azure Blob Storage or an `https://` server that needs authentication. " +
			"It applies to `package_file_source` on `jamfplatform_pro_package` and to no other attribute. " +
			"Every nested block is optional. With none set, an `s3://` source uses the AWS default credential chain, and an `azblob://` source uses the `ARM_*` environment variables and then the Azure CLI. " +
			"Values set here are not written to state. They are carried in a saved plan file. " +
			"See the [Package files from cloud storage guide](../guides/package-cloud-storage).",
		Attributes: map[string]schema.Attribute{
			"s3":    fileSourcesS3Attribute(),
			"azure": fileSourcesAzureAttribute(),
			"https": fileSourcesHTTPSAttribute(),
		},
	}
}

// fileSourcesS3Attribute is the file_sources.s3 block.
func fileSourcesS3Attribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Settings for `s3://bucket/key` sources. The argument names match Terraform's `s3` backend.",
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "AWS Region the client starts in. Can also be set with `AWS_REGION` or `AWS_DEFAULT_REGION`. A bucket in another Region is found without further configuration.",
			},
			"profile": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Named profile in the shared configuration files. Can also be set with `AWS_PROFILE`.",
			},
			"shared_config_files": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Paths of the shared configuration files. Defaults to `~/.aws/config`.",
			},
			"shared_credentials_files": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Paths of the shared credentials files. Defaults to `~/.aws/credentials`.",
			},
			"access_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "AWS access key ID. Requires `secret_key`. A role or a profile keeps the key out of the saved plan file.",
				Validators:          []validator.String{stringvalidator.AlsoRequires(sibling("secret_key"))},
			},
			"secret_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "AWS secret access key. Requires `access_key`.",
				Validators:          []validator.String{stringvalidator.AlsoRequires(sibling("access_key"))},
			},
			"token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Session token of temporary credentials. Requires `access_key`. Temporary credentials set here are not refreshed, so they must outlive the upload.",
				Validators:          []validator.String{stringvalidator.AlsoRequires(sibling("access_key"))},
			},
			"assume_role": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "IAM role to assume before reading.",
				Attributes: map[string]schema.Attribute{
					"role_arn": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "ARN of the role.",
					},
					"session_name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Name of the role session, which appears in AWS CloudTrail.",
					},
					"external_id": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "External ID the role's trust policy asks for.",
					},
				},
			},
			"assume_role_with_web_identity": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "IAM role to assume with an OpenID Connect token, the usual arrangement on a CI runner.",
				Attributes: map[string]schema.Attribute{
					"role_arn": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "ARN of the role.",
					},
					"session_name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Name of the role session, which appears in AWS CloudTrail.",
					},
					"web_identity_token": schema.StringAttribute{
						Optional:            true,
						Sensitive:           true,
						MarkdownDescription: "The OpenID Connect token. Cannot be combined with `web_identity_token_file`.",
						Validators:          []validator.String{stringvalidator.ConflictsWith(sibling("web_identity_token_file"))},
					},
					"web_identity_token_file": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Path of a file holding the OpenID Connect token. Cannot be combined with `web_identity_token`.",
					},
				},
			},
			"endpoints": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Custom service endpoints, for S3-compatible storage such as MinIO.",
				Attributes: map[string]schema.Attribute{
					"s3": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "URL of the S3 endpoint.",
					},
					"sts": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "URL of the AWS STS endpoint, used when a role is assumed.",
					},
				},
			},
			"use_path_style": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Put the bucket in the URL path (`https://host/bucket/key`) instead of the hostname. S3-compatible storage usually needs it. Defaults to `false`.",
			},
		},
	}
}

// fileSourcesAzureAttribute is the file_sources.azure block.
func fileSourcesAzureAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		MarkdownDescription: "Settings for `azblob://account/container/blob` sources. The argument names match Terraform's `azurerm` backend. " +
			"Authentication is through Microsoft Entra ID alone: a storage account key is not read, and a SAS URL is an `https://` source. " +
			"Each method is switched on separately. One is used, in this order: a client secret, `use_oidc`, `use_msi`, `use_cli`. A method that is switched on but incomplete, or that fails, is an error and does not fall through to the next.",
		Attributes: map[string]schema.Attribute{
			"tenant_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Microsoft Entra tenant ID. Can also be set with `ARM_TENANT_ID`.",
			},
			"client_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Client ID of the app registration, or of a user-assigned managed identity with `use_msi`. Can also be set with `ARM_CLIENT_ID`.",
			},
			"use_oidc": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Authenticate with an OpenID Connect token through a federated credential on the app registration. Can also be set with `ARM_USE_OIDC`. On GitHub Actions the token is requested from the runner without further configuration.",
			},
			"oidc_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "The OpenID Connect token, for `use_oidc`. Can also be set with `ARM_OIDC_TOKEN`. Cannot be combined with `oidc_token_file_path`.",
				Validators:          []validator.String{stringvalidator.ConflictsWith(sibling("oidc_token_file_path"))},
			},
			"oidc_token_file_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path of a file holding the OpenID Connect token, for `use_oidc`. Can also be set with `ARM_OIDC_TOKEN_FILE_PATH`. Cannot be combined with `oidc_token`.",
			},
			"oidc_request_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "URL the OpenID Connect token is requested from, for `use_oidc` on a CI system that issues tokens on request. Can also be set with `ARM_OIDC_REQUEST_URL` or `ACTIONS_ID_TOKEN_REQUEST_URL`.",
			},
			"oidc_request_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Bearer token that authorises the request to `oidc_request_url`. Can also be set with `ARM_OIDC_REQUEST_TOKEN` or `ACTIONS_ID_TOKEN_REQUEST_TOKEN`.",
			},
			"use_msi": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Authenticate with a managed identity. Can also be set with `ARM_USE_MSI`. It works only on compute hosted in Azure.",
			},
			"msi_endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Custom managed identity endpoint, for `use_msi`. Can also be set with `ARM_MSI_ENDPOINT`.",
			},
			"use_cli": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Authenticate with the account signed in to the Azure CLI. Can also be set with `ARM_USE_CLI`. Defaults to `true`, so it is the method used when no other is enabled.",
			},
			"client_secret": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Client secret of the app registration. Can also be set with `ARM_CLIENT_SECRET`. Needs `tenant_id` and `client_id`. Cannot be combined with `client_secret_file_path`.",
				Validators:          []validator.String{stringvalidator.ConflictsWith(sibling("client_secret_file_path"))},
			},
			"client_secret_file_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path of a file holding the client secret. Can also be set with `ARM_CLIENT_SECRET_FILE_PATH`. Cannot be combined with `client_secret`.",
			},
			"environment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Azure cloud the storage account belongs to: " + markdownValueList(filesources.AzureEnvironments()) + ". Can also be set with `ARM_ENVIRONMENT`. Defaults to `" + filesources.AzureEnvironmentPublic + "`.",
				Validators:          []validator.String{stringvalidator.OneOf(filesources.AzureEnvironments()...)},
			},
		},
	}
}

// fileSourcesHTTPSAttribute is the file_sources.https list.
func fileSourcesHTTPSAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true,
		MarkdownDescription: "Credentials for `https://` sources, one entry per server. " +
			"A credential is sent to its own host over `https` and nowhere else, and is dropped when a redirect leaves that host. " +
			"Each entry sets exactly one of `bearer_token`, `username` with `password`, or `header`.",
		Validators: []validator.List{httpsHostsValidator{}},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"host": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Required. Hostname the credential belongs to, with an optional port: `files.example.com` or `files.example.com:8443`. No scheme and no path. The port defaults to 443.",
					Validators:          []validator.String{httpsHostValidator{}},
				},
				"bearer_token": schema.StringAttribute{
					Optional:            true,
					Sensitive:           true,
					MarkdownDescription: "Token sent as `Authorization: Bearer`.",
					Validators: []validator.String{
						stringvalidator.LengthAtLeast(1),
						stringvalidator.ExactlyOneOf(sibling("username"), sibling("header")),
					},
				},
				"username": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "User name for HTTP basic authentication. Requires `password`.",
					Validators:          []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.AlsoRequires(sibling("password"))},
				},
				"password": schema.StringAttribute{
					Optional:            true,
					Sensitive:           true,
					MarkdownDescription: "Password for HTTP basic authentication. Requires `username`.",
					Validators:          []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.AlsoRequires(sibling("username"))},
				},
				"header": schema.SingleNestedAttribute{
					Optional:            true,
					MarkdownDescription: "One request header carrying the credential, for a server that reads an API key from a header of its own.",
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Header name, such as `X-Api-Key`.",
							Validators: []validator.String{
								stringvalidator.RegexMatches(headerNamePattern, "must be an HTTP header name"),
								httpsHeaderNameValidator{},
							},
						},
						"value": schema.StringAttribute{
							Required:            true,
							Sensitive:           true,
							MarkdownDescription: "Header value.",
							Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
						},
					},
				},
			},
		},
	}
}

// markdownValueList renders values as a backticked, comma-separated list, so a
// description and the OneOf validator beside it are built from one slice.
func markdownValueList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "`" + v + "`"
	}
	return strings.Join(quoted, ", ")
}

// unsendableHeaders are the request headers a credential cannot travel in,
// lower-cased. The HTTP client writes or withholds each of them itself: a
// value set for Host or one of the framing and connection headers never
// reaches the server, Referer is rewritten on every redirect, and User-Agent
// is the provider's own on every request. A credential configured under one
// of them would be dropped without a word, so the name is refused.
var unsendableHeaders = []string{
	"connection", "content-length", "host", "keep-alive", "proxy-connection",
	"referer", "te", "trailer", "transfer-encoding", "upgrade", "user-agent",
}

// httpsHeaderNameValidator refuses a custom header name the HTTP client would
// not send as given.
type httpsHeaderNameValidator struct{}

// Description returns a plain-text description of the validator.
func (httpsHeaderNameValidator) Description(_ context.Context) string {
	return "must be a header the request can carry a credential in"
}

// MarkdownDescription returns the markdown description of the validator.
func (v httpsHeaderNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (httpsHeaderNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if slices.Contains(unsendableHeaders, strings.ToLower(req.ConfigValue.ValueString())) {
		resp.Diagnostics.AddAttributeError(req.Path, "Header cannot carry a credential", fmt.Sprintf(
			"The download sets or withholds the %s header itself, so a credential in it would not reach the server. Use the header the server reads its API key from.",
			req.ConfigValue.ValueString()))
	}
}

// httpsHostValidator holds an https credential's host to the form
// filesources.NormalizeHost reads.
type httpsHostValidator struct{}

// Description returns a plain-text description of the validator.
func (httpsHostValidator) Description(_ context.Context) string {
	return "must be a hostname with an optional port, and nothing else"
}

// MarkdownDescription returns the markdown description of the validator.
func (v httpsHostValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (httpsHostValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := filesources.NormalizeHost(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid https credential host", helpers.APIErrorDetail(err))
	}
}

// httpsHostsValidator requires a host on every https entry and refuses two
// entries for one host.
//
// host is Optional in the schema and required here. The framework applies a
// Required check to the attributes of a list element even when the element is
// wholly unknown, which is what `https = [var.credential]` is under
// `terraform validate`, and reports the host as missing. Checking presence
// here lets an unknown element defer like every other unknown value.
//
// Hosts are compared after normalisation, so "files.example.com",
// "FILES.example.com" and "files.example.com:443" are one host. Two
// credentials for it would leave which one is sent to the order of the list.
// An entry whose host is unknown or malformed is skipped by the comparison:
// the first defers to apply and the second is reported by httpsHostValidator.
type httpsHostsValidator struct{}

// Description returns a plain-text description of the validator.
func (httpsHostsValidator) Description(_ context.Context) string {
	return "each entry names a host, and each host may appear once"
}

// MarkdownDescription returns the markdown description of the validator.
func (v httpsHostsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateList implements validator.List.
func (httpsHostsValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	first := map[string]int{}
	for i, element := range req.ConfigValue.Elements() {
		entry, ok := element.(types.Object)
		if !ok || entry.IsNull() || entry.IsUnknown() {
			continue
		}
		host, ok := entry.Attributes()["host"].(types.String)
		if !ok || host.IsUnknown() {
			continue
		}
		at := req.Path.AtListIndex(i).AtName("host")
		if host.IsNull() {
			resp.Diagnostics.AddAttributeError(at, "Missing https credential host",
				"Every https entry names the host its credential belongs to. Set host.")
			continue
		}
		key, err := filesources.NormalizeHost(host.ValueString())
		if err != nil {
			continue
		}
		if earlier, seen := first[key]; seen {
			resp.Diagnostics.AddAttributeError(at, "Duplicate https credential host",
				fmt.Sprintf("Entry %d already holds the credential for %s. A host takes one credential.", earlier, key))
			continue
		}
		first[key] = i
	}
}

// unknownPaths collects the file_sources paths whose values are unknown, while
// reading the known ones. Each reader returns the zero value for null and for
// unknown, and records the path only for unknown.
type unknownPaths []string

// str reads a string.
func (u *unknownPaths) str(v types.String, at string) string {
	if v.IsUnknown() {
		*u = append(*u, at)
		return ""
	}
	return v.ValueString()
}

// flag reads a bool, keeping null apart from false.
func (u *unknownPaths) flag(v types.Bool, at string) *bool {
	if v.IsUnknown() {
		*u = append(*u, at)
		return nil
	}
	return v.ValueBoolPointer()
}

// strs reads a list of strings. An unknown element makes the whole list
// unknown, since a partial list of paths would name a different set of files.
func (u *unknownPaths) strs(v types.List, at string) []string {
	if v.IsUnknown() {
		*u = append(*u, at)
		return nil
	}
	out := make([]string, 0, len(v.Elements()))
	for _, element := range v.Elements() {
		s, ok := element.(types.String)
		if !ok || s.IsUnknown() {
			*u = append(*u, at)
			return nil
		}
		out = append(out, s.ValueString())
	}
	return out
}

// block reports whether a nested object is present and known, recording it
// when it is unknown.
func (u *unknownPaths) block(v types.Object, at string) bool {
	if v.IsUnknown() {
		*u = append(*u, at)
		return false
	}
	return !v.IsNull()
}

// resolveFileSources turns the file_sources block into the plain configuration
// the cloud storage resolver is built from, and lists the paths whose values
// are unknown.
//
// A value is unknown while Terraform plans when it derives from a resource
// that does not exist yet. Configure cannot wait for it, so the resolver is
// built from what is known and told what is missing; a consumer decides
// whether its plan-time check can run without it. During apply every value is
// known and the list is empty.
//
// It reads configuration only. The stores' own environment variables are read
// by the drivers, which is why nothing here falls back to one.
func resolveFileSources(ctx context.Context, block types.Object) (filesources.Config, []string, diag.Diagnostics) {
	var (
		config  filesources.Config
		unknown unknownPaths
		diags   diag.Diagnostics
	)
	const root = "file_sources"
	if !unknown.block(block, root) {
		return config, unknown, diags
	}
	var model fileSourcesModel
	diags.Append(block.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return config, unknown, diags
	}

	if unknown.block(model.S3, root+".s3") {
		s3, s3Diags := resolveS3FileSources(ctx, model.S3, root+".s3", &unknown)
		diags.Append(s3Diags...)
		config.S3 = s3
	}
	if unknown.block(model.Azure, root+".azure") {
		azure, azureDiags := resolveAzureFileSources(ctx, model.Azure, root+".azure", &unknown)
		diags.Append(azureDiags...)
		config.Azure = azure
	}
	https, httpsDiags := resolveHTTPSFileSources(ctx, model.HTTPS, root+".https", &unknown)
	diags.Append(httpsDiags...)
	config.HTTPS = https
	return config, unknown, diags
}

// resolveS3FileSources reads the file_sources.s3 block.
func resolveS3FileSources(ctx context.Context, block types.Object, at string, unknown *unknownPaths) (filesources.S3Config, diag.Diagnostics) {
	var (
		model fileSourcesS3Model
		out   filesources.S3Config
	)
	diags := block.As(ctx, &model, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return out, diags
	}
	out.Region = unknown.str(model.Region, at+".region")
	out.Profile = unknown.str(model.Profile, at+".profile")
	out.SharedConfigFiles = unknown.strs(model.SharedConfigFiles, at+".shared_config_files")
	out.SharedCredentialsFiles = unknown.strs(model.SharedCredentialsFiles, at+".shared_credentials_files")
	out.AccessKey = unknown.str(model.AccessKey, at+".access_key")
	out.SecretKey = unknown.str(model.SecretKey, at+".secret_key")
	out.Token = unknown.str(model.Token, at+".token")
	if flag := unknown.flag(model.UsePathStyle, at+".use_path_style"); flag != nil {
		out.UsePathStyle = *flag
	}

	if unknown.block(model.AssumeRole, at+".assume_role") {
		var role fileSourcesS3AssumeRoleModel
		diags.Append(model.AssumeRole.As(ctx, &role, basetypes.ObjectAsOptions{})...)
		out.AssumeRole = &filesources.S3AssumeRole{
			RoleARN:     unknown.str(role.RoleARN, at+".assume_role.role_arn"),
			SessionName: unknown.str(role.SessionName, at+".assume_role.session_name"),
			ExternalID:  unknown.str(role.ExternalID, at+".assume_role.external_id"),
		}
	}
	if unknown.block(model.AssumeRoleWithWebIdentity, at+".assume_role_with_web_identity") {
		const here = ".assume_role_with_web_identity"
		var role fileSourcesS3WebIdentityModel
		diags.Append(model.AssumeRoleWithWebIdentity.As(ctx, &role, basetypes.ObjectAsOptions{})...)
		out.AssumeRoleWithWebIdentity = &filesources.S3AssumeRoleWithWebIdentity{
			RoleARN:              unknown.str(role.RoleARN, at+here+".role_arn"),
			SessionName:          unknown.str(role.SessionName, at+here+".session_name"),
			WebIdentityToken:     unknown.str(role.WebIdentityToken, at+here+".web_identity_token"),
			WebIdentityTokenFile: unknown.str(role.WebIdentityTokenFile, at+here+".web_identity_token_file"),
		}
	}
	if unknown.block(model.Endpoints, at+".endpoints") {
		var endpoints fileSourcesS3EndpointsModel
		diags.Append(model.Endpoints.As(ctx, &endpoints, basetypes.ObjectAsOptions{})...)
		out.EndpointS3 = unknown.str(endpoints.S3, at+".endpoints.s3")
		out.EndpointSTS = unknown.str(endpoints.STS, at+".endpoints.sts")
	}
	return out, diags
}

// resolveAzureFileSources reads the file_sources.azure block.
func resolveAzureFileSources(ctx context.Context, block types.Object, at string, unknown *unknownPaths) (filesources.AzureConfig, diag.Diagnostics) {
	var model fileSourcesAzureModel
	diags := block.As(ctx, &model, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return filesources.AzureConfig{}, diags
	}
	return filesources.AzureConfig{
		TenantID:         unknown.str(model.TenantID, at+".tenant_id"),
		ClientID:         unknown.str(model.ClientID, at+".client_id"),
		UseOIDC:          unknown.flag(model.UseOIDC, at+".use_oidc"),
		OIDCToken:        unknown.str(model.OIDCToken, at+".oidc_token"),
		OIDCTokenFile:    unknown.str(model.OIDCTokenFilePath, at+".oidc_token_file_path"),
		OIDCRequestURL:   unknown.str(model.OIDCRequestURL, at+".oidc_request_url"),
		OIDCRequestToken: unknown.str(model.OIDCRequestToken, at+".oidc_request_token"),
		UseMSI:           unknown.flag(model.UseMSI, at+".use_msi"),
		MSIEndpoint:      unknown.str(model.MSIEndpoint, at+".msi_endpoint"),
		UseCLI:           unknown.flag(model.UseCLI, at+".use_cli"),
		ClientSecret:     unknown.str(model.ClientSecret, at+".client_secret"),
		ClientSecretFile: unknown.str(model.ClientSecretFilePath, at+".client_secret_file_path"),
		Environment:      unknown.str(model.Environment, at+".environment"),
	}, diags
}

// resolveHTTPSFileSources reads the file_sources.https list. An entry with any
// unknown value is left out whole and recorded: a credential missing its host
// or its secret must not be half-applied.
func resolveHTTPSFileSources(ctx context.Context, list types.List, at string, unknown *unknownPaths) ([]filesources.HTTPSCredential, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsUnknown() {
		*unknown = append(*unknown, at)
		return nil, diags
	}
	var out []filesources.HTTPSCredential
	for i, element := range list.Elements() {
		here := fmt.Sprintf("%s[%d]", at, i)
		entry, ok := element.(types.Object)
		if !ok || entry.IsNull() {
			continue
		}
		if !unknown.block(entry, here) {
			continue
		}
		var model fileSourcesHTTPSModel
		diags.Append(entry.As(ctx, &model, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			return nil, diags
		}
		var partial unknownPaths
		credential := filesources.HTTPSCredential{
			Host:        partial.str(model.Host, here+".host"),
			BearerToken: partial.str(model.BearerToken, here+".bearer_token"),
			Username:    partial.str(model.Username, here+".username"),
			Password:    partial.str(model.Password, here+".password"),
		}
		if partial.block(model.Header, here+".header") {
			var header fileSourcesHTTPSHeaderModel
			diags.Append(model.Header.As(ctx, &header, basetypes.ObjectAsOptions{})...)
			credential.HeaderName = partial.str(header.Name, here+".header.name")
			credential.HeaderValue = partial.str(header.Value, here+".header.value")
		}
		if len(partial) > 0 {
			*unknown = append(*unknown, partial...)
			continue
		}
		out = append(out, credential)
	}
	return out, diags
}

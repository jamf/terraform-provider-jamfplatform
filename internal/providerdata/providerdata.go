// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package providerdata defines the value passed from the provider Configure phase to
// every resource, data source, list resource, and action Configure call. It is in its
// own package (rather than internal/provider) to avoid an import cycle: internal/provider
// imports every resource package for registration, so resource packages cannot import it.
package providerdata

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/aischemas"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

// ProviderMinJamfProVersion is the provider-wide recommended minimum Jamf Pro
// tenant version. Surfaces as a WARNING (not an error) when the tenant is below
// it, and is rendered to users as "Built against API as of" in the provider
// description, docs/index.md and README.md.
//
// It tracks the SDK's jamfplatform.JamfProAPIVersion — the API surface the linked
// SDK build was generated against — and is bumped with the SDK dependency. It is
// NOT the maximum of the per-resource floors: those are almost all empty (one
// resource declares 11.25.0), so a max-of-floors rule would move this backwards
// and make the rendered "built against" claim wrong.
//
// Bump at release time to match jamfplatform.JamfProAPIVersion, then run
// `make generate` for docs/index.md and hand-edit the table in README.md, which
// is not generated and will otherwise drift silently.
//
// This is deliberately advisory: it says which API the provider was built for,
// not what a tenant must run. Anything with a real requirement declares its own
// minJamfProVersion and hard-fails Configure — grep BOTH internal/resources/pro/
// and internal/actions/pro/ for those. Only one construct declares a non-empty
// floor today (service_discovery_enrollment, 11.25.0) and no action does, but
// keep checking both trees: actions have carried floors before and will again.
const ProviderMinJamfProVersion = "11.31.0"

// Data is the value passed via ResourceData/DataSourceData/ListResourceData/ActionData.
// It bundles the authenticated SDK client with lazy Jamf Pro version state shared
// across all Pro resource Configure calls in a single terraform invocation.
//
// Caching semantics:
//   - Successful version fetches are cached for the lifetime of the Data value.
//   - Errors are NOT cached — subsequent Configure calls will retry the fetch. This
//     avoids a transient network/auth blip in the first Pro Configure poisoning every
//     later Configure on the same terraform invocation.
//   - The provider-floor advisory warning is emitted at most once per Data value to
//     prevent N duplicate warnings in configs that use N Pro resources.
//   - Once the floor advisory has been considered (emitted or determined not to be
//     applicable), further Configure calls with empty minVer skip the version fetch
//     entirely — there is nothing left to check on those code paths.
//
// It also carries the API integration scope (environment, tenant, or
// organization) the provider was configured with, fixed before any construct
// runs and read via RequireScope; see scope.go.
type Data struct {
	Client *jamfplatform.Client

	scope ScopeKind

	proMu      sync.Mutex
	proVersion string

	floorMu      sync.Mutex
	floorHandled bool

	onceMu    sync.Mutex
	onceFired map[string]struct{}

	// enrollmentWriteMu serializes writes to the shared Jamf Pro enrollment-settings
	// backing store. The /v4/enrollment object and the /v1/reenrollment object are two
	// views of ONE record: a write to either propagates to the other's read, and the
	// /v4 PUT is full-replace (it must round-trip every field it does not change). The
	// jamfplatform_pro_user_initiated_enrollment_settings resource does a read-merge-write
	// against /v4; jamfplatform_pro_re_enrollment_settings writes /v1. Terraform applies
	// resources with no dependency edge concurrently (e.g. both Created on first apply),
	// so without serialization one resource's read-merge-write can clobber the other's
	// write from a stale read. Both resources lock this mutex around their entire
	// read→modify→write critical section. NOTE: this only serializes within a single
	// provider process; two separate `terraform apply` runs against the same tenant can
	// still race (EnrollmentSettingsV4 carries no version/ETag for optimistic concurrency).
	enrollmentWriteMu sync.Mutex

	// versionFetcher is the function used to retrieve the tenant Jamf Pro version.
	// Tests override this to avoid real HTTP calls. Nil means use the default SDK path.
	versionFetcher func(ctx context.Context) (string, error)

	// impactCache backs the plan-time impact alerts. Nil when the provider's
	// impact_alerts attribute is unset, which is the default — a nil cache
	// reports nothing, so resources need no flag check of their own.
	impactCache *impact.Cache

	aiSchemaMu    sync.Mutex
	aiSchemaCache *aischemas.Cache

	patchSourceMu    sync.Mutex
	patchSourceCache *PatchSourceCache

	appTitleMu    sync.Mutex
	appTitleCache *AppTitleCatalogCache
}

// AISchemaCache returns the shared AI Governance product catalogue and vendor schema cache, building
// it on first use. Unlike the impact cache this is not behind a provider flag: it backs validation
// the AI Governance policy resource always performs, and it costs nothing until something asks it
// for a schema.
//
// One cache per configured provider instance is the point. The Claude Code schema alone is 184 KB,
// and every policy in a configuration would otherwise fetch its own copy on every plan.
func (d *Data) AISchemaCache() *aischemas.Cache {
	if d == nil {
		return nil
	}
	d.aiSchemaMu.Lock()
	defer d.aiSchemaMu.Unlock()

	if d.aiSchemaCache == nil {
		d.aiSchemaCache = aischemas.NewCache(d.Client)
	}
	return d.aiSchemaCache
}

// PatchSourceCatalogues is a snapshot of the tenant's two Jamf Pro patch source
// catalogues, internal and external, as one value.
//
// It carries the catalogue entries as read rather than a name → id index, because the
// question its consumer asks is not a plain lookup: a patch software title names its
// source but never numbers it, a name present in both catalogues cannot be resolved at
// all, and the refusal has to name the candidate ids. Keeping the snapshot as-read leaves
// that decision in one pure function over these two slices (in the patch software title
// package, which owns the law and the calls that fill this), so the cache only ever
// answers "what does this tenant have", never "which id is it".
type PatchSourceCatalogues struct {
	Internal []proclassic.IDName
	External []proclassic.IDName
}

// PatchSourceCache holds one PatchSourceCatalogues snapshot per configured provider
// instance, read on first use.
//
// Both catalogues are tenant-global and identical for every patch software title in a
// configuration, and every title read outside the managed-resource steady state needs
// them, so without this a configuration with N patch software title data sources pays 2N
// identical catalogue requests on every plan and again on every apply.
//
// A failed read is not cached: the next caller retries it. A transient blip or a
// momentary privilege problem on the first title must not blank every later title's
// source_id for the rest of the run — the same rule this package applies to the Jamf Pro
// version fetch and aischemas to its vendor schemas.
//
// The read itself is injected rather than written here. The two SDK calls it makes belong
// to the patch software title package: that package declares the privileges they require,
// and its tests derive that declaration from the call sites in its own files.
type PatchSourceCache struct {
	client *proclassic.Client
	read   func(context.Context, *proclassic.Client) (PatchSourceCatalogues, error)

	mu         sync.Mutex
	loaded     bool
	catalogues PatchSourceCatalogues
}

// Catalogues returns the snapshot, reading it at most once per configured provider
// instance.
//
// The lock is held across the read, so concurrent callers collapse into one round-trip
// instead of racing to fill the same snapshot. There is exactly one snapshot to fill —
// unlike the AI Governance schema cache, where a lock held across a fetch would make
// callers wanting different schemas wait on each other.
//
// Nil-receiver-safe: resolving a patch source name is best-effort everywhere except
// import, so a construct that never received a cache must report an unresolved id rather
// than panic mid-plan.
func (c *PatchSourceCache) Catalogues(ctx context.Context) (PatchSourceCatalogues, error) {
	if c == nil {
		return PatchSourceCatalogues{}, errors.New("the provider is not configured, so the patch source catalogues cannot be read")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loaded {
		return c.catalogues, nil
	}
	catalogues, err := c.read(ctx, c.client)
	if err != nil {
		return PatchSourceCatalogues{}, err
	}
	c.catalogues = catalogues
	c.loaded = true
	return c.catalogues, nil
}

// ConfigurePatchSources returns the patch source cache shared by every construct
// configured from this provider instance, building it on first use with read.
//
// It mirrors ConfigureImpact: no diagnostics, and nil when providerData is not a *Data —
// including the nil ProviderData the framework passes during early lifecycle. A caller
// that receives nil still resolves nothing and reports it, because a null source_id
// outside import is advisory.
//
// The first caller's read function is the cache's read function; later callers reuse the
// cache they find. Every caller passes the same package-level function, so which one
// registered it cannot matter.
func ConfigurePatchSources(providerData any, read func(context.Context, *proclassic.Client) (PatchSourceCatalogues, error)) *PatchSourceCache {
	pd, ok := providerData.(*Data)
	if !ok {
		return nil
	}
	return pd.patchSources(read)
}

// patchSources lazily builds the provider-instance patch source cache over a classic
// client of this Data's own, so no caller has to hand one in.
func (d *Data) patchSources(read func(context.Context, *proclassic.Client) (PatchSourceCatalogues, error)) *PatchSourceCache {
	if d == nil {
		return nil
	}
	d.patchSourceMu.Lock()
	defer d.patchSourceMu.Unlock()

	if d.patchSourceCache == nil {
		d.patchSourceCache = &PatchSourceCache{client: proclassic.New(d.Client), read: read}
	}
	return d.patchSourceCache
}

// AppTitleCatalogCache holds one snapshot of the tenant's Jamf App Catalog title
// list per configured provider instance, read on first use.
//
// The catalog is tenant-global and identical for every App Installer in a
// configuration, and each deployment needs it in both directions — a configured
// app_title_name resolved to a catalog id at plan time and again on apply, and the
// stored app_title_id reverse-resolved to its display name on every refresh. Without
// this a configuration with N App Installers pays 2N catalog requests on every plan
// on top of the N deployment reads, tripling the request count of a no-op plan. One
// unfiltered list answers all of it: the catalog is a few hundred titles and the
// SDK's list call pages at 2000, so the whole of it arrives in one round-trip.
//
// A failed read is not cached: the next caller retries it. A transient blip or a
// momentary privilege problem on the first deployment must not blank every later
// deployment's app_title_name for the rest of the run — the same rule this package
// applies to the Jamf Pro version fetch and to the patch source catalogues.
//
// The read itself is injected rather than written here. The SDK call it makes belongs
// to the App Installer package: that package declares the privileges it requires, and
// its tests derive that declaration from the call sites in its own files.
type AppTitleCatalogCache struct {
	client *pro.Client
	read   func(context.Context, *pro.Client) ([]pro.AppTitle, error)

	mu     sync.Mutex
	loaded bool
	titles []pro.AppTitle
}

// Titles returns the catalog snapshot, reading it at most once per configured
// provider instance.
//
// The lock is held across the read, so concurrent callers collapse into one
// round-trip instead of racing to fill the same snapshot — the same shape as
// PatchSourceCache.Catalogues, and for the same reason: there is exactly one
// snapshot to fill.
//
// Nil-receiver-safe: resolving a title name is a plan-time preflight and a
// best-effort refresh everywhere except import, so a construct that never received
// a cache must report the failure rather than panic mid-plan.
func (c *AppTitleCatalogCache) Titles(ctx context.Context) ([]pro.AppTitle, error) {
	if c == nil {
		return nil, errors.New("the provider is not configured, so the App Catalog titles cannot be read")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loaded {
		return c.titles, nil
	}
	titles, err := c.read(ctx, c.client)
	if err != nil {
		return nil, err
	}
	c.titles = titles
	c.loaded = true
	return c.titles, nil
}

// ConfigureAppTitleCatalog returns the App Catalog title cache shared by every
// construct configured from this provider instance, building it on first use with
// read.
//
// It mirrors ConfigurePatchSources: no diagnostics, and nil when providerData is not
// a *Data — including the nil ProviderData the framework passes during early
// lifecycle. A caller that receives nil resolves nothing and reports it, which is
// the same outcome an unconfigured client already produced.
//
// The first caller's read function is the cache's read function; later callers reuse
// the cache they find. Every caller passes the same package-level function, so which
// one registered it cannot matter.
func ConfigureAppTitleCatalog(providerData any, read func(context.Context, *pro.Client) ([]pro.AppTitle, error)) *AppTitleCatalogCache {
	pd, ok := providerData.(*Data)
	if !ok {
		return nil
	}
	return pd.appTitleCatalog(read)
}

// appTitleCatalog lazily builds the provider-instance App Catalog title cache over a
// Pro client of this Data's own, so no caller has to hand one in.
func (d *Data) appTitleCatalog(read func(context.Context, *pro.Client) ([]pro.AppTitle, error)) *AppTitleCatalogCache {
	if d == nil {
		return nil
	}
	d.appTitleMu.Lock()
	defer d.appTitleMu.Unlock()

	if d.appTitleCache == nil {
		d.appTitleCache = &AppTitleCatalogCache{client: pro.New(d.Client), read: read}
	}
	return d.appTitleCache
}

// EnableImpactAlerts turns on plan-time impact alerts for this provider
// instance, backed by one shared tenant read. Called from provider Configure
// when the impact_alerts attribute is set.
func (d *Data) EnableImpactAlerts() {
	d.impactCache = impact.NewTenantCache(d.Client)
}

// ImpactCache returns the shared impact cache, or nil when impact alerts are
// off. Resources pass the result straight into impact.Report, which treats nil
// as disabled. Nil-receiver-safe: impact reporting is advisory, so a resource
// whose Configure never ran must degrade to disabled, not panic mid-plan.
func (d *Data) ImpactCache() *impact.Cache {
	if d == nil {
		return nil
	}
	return d.impactCache
}

// ConfigureImpact returns the shared impact cache from providerData, or nil when
// impact alerts are off or providerData is not yet available. It never produces
// diagnostics: impact reporting is advisory and must not affect whether a
// resource configures successfully.
func ConfigureImpact(providerData any) *impact.Cache {
	pd, ok := providerData.(*Data)
	if !ok {
		return nil
	}
	return pd.ImpactCache()
}

// EnrollmentWriteLock returns the process-shared mutex that serializes writes to the
// shared enrollment-settings backing store. Both the user-initiated-enrollment-settings
// (/v4) and re-enrollment-settings (/v1) resources must lock it around their entire
// read→modify→write critical section. Because Data is shared by pointer across every
// resource's Configure, all callers receive the same *sync.Mutex instance.
func (d *Data) EnrollmentWriteLock() *sync.Mutex {
	return &d.enrollmentWriteMu
}

// New wraps a configured SDK client in a Data value, reading the API integration
// scope back off the client so it cannot disagree with the scope option the
// client was actually built with.
//
// SDK v0.18.0 added Client.Scope() for exactly this; before it the scope had to
// be passed in alongside, because the transport exposed only TenantID() — which
// returns "" for both environment and organization scope and so cannot tell them
// apart — and its scope type was unreachable in internal/client.
func New(client *jamfplatform.Client) *Data {
	return &Data{Client: client, scope: scopeFromClient(client)}
}

// scopeFromClient maps the SDK's scope onto the provider's ScopeKind.
//
// The provider keeps its own enum rather than aliasing the SDK's because its
// kinds carry user-facing wording: a diagnostic has to tell a practitioner which
// integration scope they configured and which attribute or environment variable
// selected it, none of which belongs in an SDK log field. Both enums now name
// organization as their zero value and render it the same way — SDK v0.22.0
// corrected String() from "none", which had named a fourth state the model does
// not have — so the two agree on the vocabulary they share. What they do about a
// kind they do not share is scopeKindFromClient's business, below; scopes.go
// answers the same question for a registry entry.
//
// A nil client is organization scope and is reached in normal use: the framework
// calls Configure with a nil ProviderData during early lifecycle, so this must
// not dereference it.
func scopeFromClient(c *jamfplatform.Client) ScopeKind {
	if c == nil {
		return ScopeOrganization
	}
	return scopeKindFromClient(c.Scope())
}

// scopeKindFromClient maps the scope a configured client reports — the pair
// Client.Scope() returns — onto the provider's ScopeKind. It is the client-side
// twin of scopeKindFromSDK, which does the same job for a registry entry.
//
// An empty ID is organization scope, and legitimately so: sending no scope
// header is how an organization-scoped credential works, because the gateway
// then resolves the context from the access token alone.
//
// A kind that is none of the three panics rather than falling through, and the
// direction of the fallback is why. ScopeOrganization is the sole member of
// AccountScopes, so it is the one value that *passes* the gate in
// ConfigureAccount — the gate whose whole purpose is to admit only a
// header-less organization credential. Defaulting to it would fail closed at
// every other call site and fail open at the one where it counts, which is the
// failure the derived scope sets exist to make impossible.
//
// The panic is deliberate in preference to a sentinel kind that no family set
// contains, even though this runs at provider Configure in a live plugin
// process, where a panic reaches the operator as an opaque go-plugin crash
// rather than a diagnostic. Two reasons. No practitioner input can reach the
// branch: the kind originates in the scope option the provider itself handed the
// SDK, so a fourth kind arrives only once this repository grows an option that
// selects one — and the first plan against such a build panics deterministically,
// in development, naming what to add. And a sentinel would be reported as
// "organization" by ScopeKind.String, which has no case for it, so its
// fail-closed diagnostic would tell an operator they had configured a scope they
// had not — the same conflation the SDK removed from its own String in v0.22.0.
//
// What the panic guards is bounded, because it sits below the empty-ID branch:
// Client.Scope() returns an empty ID for every kind carrying no request header,
// so a fourth kind the SDK grows without a header still resolves to organization
// above. Only a header-bearing one the provider has no ScopeKind for lands here.
func scopeKindFromClient(kind jamfplatform.ScopeKind, id string) ScopeKind {
	if id == "" {
		return ScopeOrganization
	}
	switch kind {
	case jamfplatform.ScopeEnvironment:
		return ScopeEnvironment
	case jamfplatform.ScopeTenant:
		return ScopeTenant
	case jamfplatform.ScopeOrganization:
		return ScopeOrganization
	}
	panic(fmt.Sprintf("providerdata: the configured SDK client reports scope kind %d, which this "+
		"provider has no ScopeKind for — add one rather than letting it default to organization, "+
		"the only scope the jamfplatform_account_* gate accepts", kind))
}

// GetJamfProVersion fetches the tenant's Jamf Pro version. Successful results are
// cached for the lifetime of the Data value. Errors are not cached — the next call
// retries the fetch. Resources with empty minJamfProVersion should not call this —
// fetching only fires when a Pro resource with a version requirement is in the config.
func (d *Data) GetJamfProVersion(ctx context.Context) (string, error) {
	d.proMu.Lock()
	defer d.proMu.Unlock()
	if d.proVersion != "" {
		return d.proVersion, nil
	}
	fetch := d.versionFetcher
	if fetch == nil {
		fetch = d.defaultVersionFetch
	}
	v, err := fetch(ctx)
	if err != nil {
		return "", err
	}
	d.proVersion = v
	return v, nil
}

// defaultVersionFetch is the production version fetcher backed by the Jamf Pro SDK.
func (d *Data) defaultVersionFetch(ctx context.Context) (string, error) {
	v, err := pro.New(d.Client).GetJamfProVersionV1(ctx)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return v.Version, nil
}

// providerFloorWarning returns a warning diagnostic if d.proVersion is below
// ProviderMinJamfProVersion. Fires at most once per Data value: the first call
// records the consideration (whether or not a warning is emitted) and every later
// call returns nil so configs with many Pro resources do not surface duplicate
// warnings. The caller must have invoked GetJamfProVersion before calling this so
// d.proVersion is populated.
func (d *Data) providerFloorWarning() diag.Diagnostic {
	d.floorMu.Lock()
	defer d.floorMu.Unlock()
	if d.floorHandled {
		return nil
	}
	d.floorHandled = true
	if d.proVersion == "" {
		return nil
	}
	return helpers.WarnIfBelowProviderFloor(d.proVersion, ProviderMinJamfProVersion)
}

// floorAlreadyHandled reports whether the provider-floor advisory has already been
// considered for this Data value. Used by ConfigurePro to short-circuit the version
// fetch on resources with empty minVer once the floor has been emitted or skipped.
func (d *Data) floorAlreadyHandled() bool {
	d.floorMu.Lock()
	defer d.floorMu.Unlock()
	return d.floorHandled
}

// FiredOnce records and reports whether an event keyed by name has already been
// handled for this Data value. Returns true the first time it is called for a
// given key (meaning: "caller should fire"); returns false on every subsequent
// call. Used by cross-API bridging paths (e.g. the device_group jamf_pro_id
// lookup) to emit a single warning per provider invocation even when the same
// failure is encountered across many resource Read/Create/Update calls.
//
// Keys are namespaced by callers — use a stable, descriptive identifier such as
// "device_group.jamf_pro_id.forbidden" so distinct latches do not collide.
func (d *Data) FiredOnce(key string) bool {
	d.onceMu.Lock()
	defer d.onceMu.Unlock()
	if d.onceFired == nil {
		d.onceFired = make(map[string]struct{})
	}
	if _, seen := d.onceFired[key]; seen {
		return false
	}
	d.onceFired[key] = struct{}{}
	return true
}

// configureSub is the shared core for every SDK sub-client Configure helper. It
// type-asserts providerData into *Data, fetches the Jamf Pro tenant version
// (lazily, cached on the Data value), runs the per-resource minimum version gate
// when minVer is non-empty, surfaces the provider-floor advisory warning when
// the tenant is below the provider build target, and constructs the sub-client
// via the supplied factory.
//
// Once the floor warning has been considered for the Data value, subsequent
// Configure calls with empty minVer skip the version fetch entirely — there is
// nothing left to evaluate on those code paths, so the network round-trip is
// avoided.
//
// The credential-scope gate is applied here once for every Jamf Pro construct
// rather than per package: the Pro API is reachable under either a tenant- or an
// environment-scoped credential, and has no account-level surface that answers
// without a scope header, so an organization-scoped credential is rejected.
//
// Returns (nil, nil) when providerData is nil (the framework calls Configure with
// a nil ProviderData during early lifecycle — that is not an error, the resource
// simply remains unconfigured until a later Configure call provides the data).
func configureSub[T any](
	ctx context.Context,
	providerData any,
	minVer, resourceType string,
	factory func(*jamfplatform.Client) *T,
) (*T, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	pd, ok := providerData.(*Data)
	if !ok {
		diags.AddError(
			"Unexpected Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, diags
	}

	if scopeDiags := pd.RequireScope(resourceType, ProScopes...); scopeDiags.HasError() {
		diags.Append(scopeDiags...)
		return nil, diags
	}

	client := factory(pd.Client)

	// Fast path: empty per-resource minVer and the provider-floor advisory has already
	// been considered for this Data value → nothing left to fetch.
	if minVer == "" && pd.floorAlreadyHandled() {
		return client, diags
	}

	version, err := pd.GetJamfProVersion(ctx)
	if err != nil {
		if minVer == "" {
			return client, diags
		}
		diags.AddError(
			"Failed to read Jamf Pro tenant version",
			fmt.Sprintf("%s requires Jamf Pro; could not read version: %s", resourceType, helpers.APIErrorDetail(err)),
		)
		return nil, diags
	}
	if minVer != "" {
		diags.Append(helpers.RequireMinJamfProVersion(version, minVer, resourceType)...)
	}
	if warn := pd.providerFloorWarning(); warn != nil {
		diags.Append(warn)
	}
	return client, diags
}

// ConfigurePro is the shared Configure boilerplate for every Pro resource, data
// source, list resource, and action. It type-asserts providerData into *Data,
// fetches the Jamf Pro tenant version (lazily, cached on the Data value), runs
// the per-resource minimum version gate when minVer is non-empty, and surfaces
// the provider-floor advisory warning when the tenant is below the provider
// build target.
//
// resourceType is the fully-qualified Terraform type name used in diagnostic
// messages (e.g. "jamfplatform_pro_category"). Returns a *pro.Client ready for
// use; callers should check resp.Diagnostics.HasError() before using it.
//
// Returns (nil, nil) when providerData is nil — the framework calls Configure
// with a nil ProviderData during early lifecycle and that is not an error; the
// resource simply remains unconfigured until a later Configure call provides
// the data.
//
// Once the floor warning has been considered for a Data value, subsequent
// Configure calls with empty minVer skip the version fetch entirely.
func ConfigurePro(ctx context.Context, providerData any, minVer, resourceType string) (*pro.Client, diag.Diagnostics) {
	return configureSub(ctx, providerData, minVer, resourceType, pro.New)
}

// ConfigureProClassic is the shared Configure boilerplate for every ProClassic
// resource, data source, list resource, and action. Semantics are identical to
// ConfigurePro — same Data value, same version cache, same one-shot floor
// warning, same nil-providerData and minVer contracts — but the returned client
// speaks XML against the classic API surface.
func ConfigureProClassic(ctx context.Context, providerData any, minVer, resourceType string) (*proclassic.Client, diag.Diagnostics) {
	return configureSub(ctx, providerData, minVer, resourceType, proclassic.New)
}

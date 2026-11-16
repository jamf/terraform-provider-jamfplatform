# Descriptions and example prose

What a schema description and an example say, and how they say it. Part of the [style guide](../STYLE_GUIDE.md).

## User-facing descriptions are UI-aligned, not wire-aligned

`MarkdownDescription` / `Description` strings on every schema attribute, resource, data source, list resource and action, in every family, are **user-facing Terraform Registry documentation**. They render in the registry and in the IDE schema browser, read side by side with the product's admin UI. The provider exists to abstract the API plumbing, and a description reflects that abstraction.

**Strip from user-facing descriptions.** Where the maintainer-side detail is still useful, move it to a doc comment: the package doc comment for a wire-name mapping ([schema-design.md §Attribute names](schema-design.md#attribute-names-mirror-the-jamf-pro-admin-ui-when-the-wire-name-is-cryptic)), or the doc comment on the constant or function concerned. Not a comment beside the attribute, because a schema is built inside a function body and [§Go Conventions](package-layout.md#go-conventions) forbids comments there.

- `<xml_tag>` references and any other wire-shape literals (e.g. `<level>`, `<jss_users>`, `<security><password>`).
- "Wire field …" / "wire field …" / "wire emits …" / "wire-symmetric" / "on the wire" / "wire shape" / "wire-canonical" framing.
- "classic API" / "classic endpoint" mentions, and endpoint paths (`/api/proclassic/...`, `/api/pro/v1/...`, `/JSSResource/...`).
- HTTP method names (`POST`, `PUT`, `GET`, `DELETE`) — rephrase as `create`/`update`/`read`/`delete` if the action matters to the user.
- SDK package names, SDK function names, Go type names, internal helper names.
- "Server-derived" — replace with `Returned by Jamf Pro; not user-settable` or `Read-only`.

**Keep — but rephrase without wire jargon**:

- Write-only / read-back quirks (e.g. *"Jamf Pro does not echo this value back after it is set, so the provider treats it as write-only."*).
- Field conflicts and validator notes (*"conflicts with X if Y"*).
- Special-case behaviour (`-1 means none`, `0 disables`, valid value lists).
- Cross-field relationships (*"Pair with X to set Y"*).
- Mode-dependent or singleton-resource notes (e.g. SSO OIDC vs SAML, tenant-wide singletons).

**Endpoint references and SDK annotations** belong in maintainer-side surfaces only:

- The `// SDK endpoints used:` annotation block at the top of every Pro/ProClassic resource's `crud.go` ([endpoint-lifecycle.md](endpoint-lifecycle.md#tracking--in-code-annotation-pro-resources-only)).
- Go package, file and function doc comments.
- Never in `MarkdownDescription` strings.

The reference implementations for the tone are `internal/resources/pro/macos_configuration_profile/resource.go` and `internal/resources/pro/mobile_device_configuration_profile/resource.go`. Copy that voice for new resources.

### Translating UI labels/presets to wire values

When the UI exposes a control as a fixed dropdown of presets but the wire stores a raw value (e.g. LAPS `rotation_interval` "30 days" ↔ `2592000` seconds), model the attribute as a `OneOf` enum of the **UI labels** and own the label↔value translation in the input/state builders (a `map[label]value` + its programmatic inverse). Reference: `internal/resources/pro/local_admin_password_settings/`.

Two rules for these tables:

- **Derive the documented value list from the same slice the `OneOf` validator uses** (a small `markdownValueList`-style helper), so the `MarkdownDescription` and the validator cannot drift apart. The provider description does the same with `ProviderMinJamfProVersion`: one constant, interpolated.
- **Acceptance cannot verify the table.** Writing through the map and reading through its inverse round-trips *by construction*, so a wrong-but-consistent entry (`"30 days"` mapped to the wrong number) passes every unit and acceptance test silently. Anchor at least one entry to the live wire during the build, and **wire-probe the table by driving the actual admin UI** (set each preset in Jamf, GET the stored value) — the round-trip test is not a substitute. Flag any unverified entries in the PR.

## Description and example prose: the terse-reference voice

The section above governs *what* a description may say. This one governs *how* it reads. It
applies to every `Description` / `MarkdownDescription` string in a schema — resource, data
source, list resource, action, function, provider configuration — and equally to the `#`
comments in `examples/**/*.tf`, which a user reads side by side with them on the Registry page.

The voice is terse reference: short declarative sentences, the fact first and each quirk in a
sentence of its own. Vary sentence length, so the text does not read as a template somebody
filled in.

Six habits account for most of the prose this provider has had to rewrite.

- **One em dash per description at most, and only for a genuine interruption.** An em dash
  gluing an appositive or a second clause onto a sentence is the strongest single signal that a
  description was generated rather than written. `Manages Jamf Pro SMTP settings (Settings →
  System → SMTP Server) — the outbound relay …` is a comma or a full stop wanting to happen.
  Two in one description is one too many, and three is never right.
- **No bolded run-in label opening a paragraph.** `**Omit = preserve** — each toggle you omit
  keeps its current value` says no more than `A toggle you omit keeps its current Jamf Pro
  value.`, in more words and in a textbook voice. Where a long resource description really does
  change topic, use a `###` heading or a plain lead sentence. Two exceptions: the bold
  quoted admin-UI label a renamed attribute must open with
  ([schema-design.md](schema-design.md#attribute-names-mirror-the-jamf-pro-admin-ui-when-the-wire-name-is-cryptic)),
  and `Requires **X API** access.`
- **Do not repeat a stock clause verbatim across packages.** `One record per tenant.` carries
  what `Singleton — one record per tenant.` did, and the next resource need not phrase it the
  same way.
- **En dash for numeric ranges** (`1–100`, `0–65535`); hyphen for everything else.
- **No throat-clearing.** Delete `Note that`, `Note:`, `NOTE:`, `IMPORTANT:`, `simply`, `just`.
  A description is already the place for the fact, so state it.
- **No padding verbs.** `is used to select` is `selects`. `used for` either names the use or goes.

Vocabulary that never earns its place in a description: *leverage, robust, seamless,
comprehensive, ensure, facilitate, streamline, delve, crucial, foster, showcase, underscore,
pivotal,* and *landscape* used figuratively. Nor do the rhetorical shapes that travel with them:
`not X, but Y`, a reflexive stack of three adjectives, or a closing sentence that restates the
description.

**Write as Jamf, not about Jamf.** This provider is Jamf's own, so its user-facing text does
not refer to Jamf in the third person, as a vendor the provider merely integrates with. Where a
sentence names the actor in an exchange, name the product or the service: *Jamf Pro refuses the
write*, *Jamf Account stores a claimed domain in lower case*, *Jamf Security Cloud enforces the
reference on delete*, *the platform compares the settings*. A bare "Jamf" as the actor is both
distancing and imprecise across five namespaces, and "Jamf's own", "the vendor" and "the API" are
the same tic in plainer form. Three things the rule does not touch: **the server** where the
server is one the reader runs (an LDAP directory, an SMTP relay, a distribution point, an external
patch source), **the API integration**, which is the reader's own, and a deliberate
product-versus-provider contrast, because "the deprecation is Jamf Pro's, not the provider's" is
the distinction a reader needs rather than a distancing tic. Contributor-facing text — Go
comments, this guide, `CONTRIBUTING.md` — is exempt.

**Pre-PR grep gate.** Before opening a PR, run the description grep in [CONTRIBUTING.md](../CONTRIBUTING.md#description-check) over the diff. It matches the habits and vocabulary above. A hit is not automatically a defect, but every hit needs a reason.

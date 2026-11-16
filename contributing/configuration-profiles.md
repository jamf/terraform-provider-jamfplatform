# Configuration profile payloads

Payload diff suppression (mask-and-compare), storage categories and the import fidelity gate. Part of the [style guide](../STYLE_GUIDE.md).

## Configuration profile payload diff suppression (mask-and-compare)

Jamf classic configuration-profile endpoints (`osxconfigurationprofiles`, `mobiledeviceconfigurationprofiles`) re-serialise the user-supplied `.mobileconfig` plist server-side: whitespace stripping, version normalisation (`1.0` → `1`), server-stamped defaults, top-level UUID/Identifier rewrites, per-payload display-name defaults keyed on `PayloadType`. Trying to *predict* every mutation produces a brittle map of `PayloadType` → server-default lookups that drifts the moment Jamf changes a default. The provider uses **mask-and-compare** instead.

**Strategy.** The plan modifier parses both sides (user input + server-canonical state) and runs the same `payloadhelpers.MaskPayload` mask across both before comparing. The mask is symmetric and content-blind — the provider never needs to know that `com.apple.notificationsettings` defaults to `"Notifications Payload"`; both sides drop the key, both sides agree.

Mask drops (or empty-normalises) from **both** sides:

- **Top-level always-clobbered**: `PayloadDisplayName` (set from `general.name`), `PayloadIdentifier`, `PayloadUUID` (Jamf assigns lowercase UUIDs).
- **Top-level server-added defaults**: `PayloadEnabled`, `PayloadDescription`, `PayloadRemovalDisallowed`.
- **`PayloadContent[i]` server-augmented**: `PayloadDisplayName`, `PayloadUUID`, `PayloadIdentifier`, `PayloadOrganization`, `PayloadEnabled`, `PayloadDescription`, `AllowUserOverrides`, `VendorConfig`.
- **String values**: recursive leading/trailing whitespace trim (Jamf strips whitespace inside e.g. `Rules[].Comment`).
- **Empty-string normalisation**: `""` on either side compares equal to "absent".
- **Line endings inside string values**: `\r\n` and lone `\r` normalise to `\n` (`normalizeLineEndings`). See below.

**Line-break tolerance (paired with the SDK's CR-reference handling).** CR is the only whitespace character Jamf Pro keeps inside a string value — literal LF and TAB are **deleted outright** (words merge) in the payload types stored verbatim and in every slot outside `PayloadContent` — and it can only be authored as a `&#13;` reference, since XML 1.0 §2.11 rewrites a literal CR to LF in transit. Hence Jamf's own admin UI emits `&#13;`, and the SDK transmits those references bare (`escapeAmpPreservingCRRefs`) rather than decoding them. It always reads back as LF though: MCX and mobile fragments normalise on store, and a verbatim-stored CR returns as a raw CR byte our own parse normalises. **Both halves are required** — this normalisation must ship with (or before) any SDK that preserves CR references, or every `&#13;` fails the fidelity gate.

Cost: CR-vs-LF drift fidelity everywhere. Accepted over a per-`PayloadType` model of the server's line-break handling, which is empirical and drifts each release. U+2028/U+2029/U+0085 are deliberately **not** normalised — they round-trip byte-exact, render as line breaks on-device, and keep full drift detection.

**`PayloadContent` entry order is not preserved on store — canonicalise it, do not compare it.** Jamf Pro **stably partitions** the top-level `PayloadContent` array into the entries it stores *verbatim* followed by the entries it *re-renders* (the same two categories as §"Payload storage categories" below), keeping relative order within each block. Wire-probed 2026-08-11 against Jamf Pro 11.30.x across 18 profile shapes, no exceptions:

| Authored order | Stored order |
|---|---|
| `[MCX, certificate]` | `[certificate, MCX]` |
| `[certificate, MCX]` | unchanged — already canonical |
| `[MCX, certificate, MCX]` | `[certificate, MCX, MCX]` |
| `[notificationsettings, MCX]` | unchanged — both re-rendered |
| `[loginwindow, certificate]` | unchanged — both verbatim |
| `[MCX, loginwindow, certificate]` | `[loginwindow, certificate, MCX]` |
| `[MCX-a, certificate, MCX-b, certificate-b, MCX-c]` | `[certificate, certificate-b, MCX-a, MCX-b, MCX-c]` |

A positional array compare therefore pairs unrelated entries and reports cascading false drift on every key of both — which reached a user as a bogus `"Jamf Pro cannot store this payload faithfully"` error plus a create-time rollback for a payload the server had stored perfectly. `maskPayloadContent` closes this by **stable-sorting the array by `PayloadType`** (`canonicalisePayloadContentOrder`), so both sides reach the comparator in the same order and every comparator built on `MaskPayload` — the lenient `LenientEqualPlist` and the strict `structuralEqual` behind `ThreeWayCompare` — inherits the fix.

Two properties make this a canonicalisation rather than a tolerance:

- **Entry order carries no meaning.** Apple treats each `PayloadContent` entry as an independent payload, so an order-insensitive comparison is the correct one.
- **The sort is stable and keyed only on `PayloadType`**, so same-type siblings keep their authored order relative to each other and an edit to one of them still surfaces as drift. That granularity is exactly what the wire law needs: same `PayloadType` implies the same partition, so Jamf Pro *cannot* reorder same-type siblings.

Do not reach for `PayloadIdentifier` as a pairing key — it is masked (server-assignable) and already gone by compare time.

The same reorder breaks the **read-back diagnostic**, which quotes indexed paths (`PayloadContent[1].Foo`) and so needs both sides to agree on which entry index 1 is. `PayloadFidelityErrorDetail` permutes the stored array back into the *authored* order (`alignPayloadContentOrder`) rather than sorting both, so the indices it prints match the payload the operator wrote.

**Web clip icons are compared after normalising, not byte-for-byte.** A `com.apple.webClip.managed` entry's `Icon` is the one image in Apple's payload schema (Release-v26.4 declares thirteen `data`-typed keys; the other twelve are certificates, a font, two VPN secrets and an SSO group identifier), and Jamf Pro **always** re-renders it — wire-probed 2026-09-09 against Jamf Pro 11.3x on both `mobiledeviceconfigurationprofiles` and `osxconfigurationprofiles`:

| Authored | Stored |
|---|---|
| 8x8 PNG | 180x180 PNG (upscaled) |
| 120x120 PNG | 180x180 PNG |
| 180x180 PNG | same dimensions, different bytes |
| 200x100 PNG | 180x90 PNG — longest side 180, aspect preserved |
| JPEG or GIF | PNG |
| anything undecodable, including an empty blob | a fixed 180x180 placeholder PNG |

The stored form keeps the source colour model and carries only `IHDR`/`IDAT`/`IEND` — every ancillary chunk is stripped. The re-render is **deterministic**, which is why an icon read back out of Jamf Pro and re-submitted does round-trip byte-exact; it is also why the failure was latent until something triggered a write, and then failed *every* write to that profile (issue #418). Reproducing the re-render client-side would mean matching another encoder's zlib settings and filter choices, so `LenientEqualPlist` compares that one leaf through `iconsEquivalent`: both sides are decoded, box-filtered to a 16x16 grid, and compared on mean per-channel difference against a tolerance of 8 (measured on the same probe run: Jamf Pro's own re-render of an icon scores 0.65–2.38, two different icons score 45.1, so the threshold sits an order of magnitude above the noise and a factor of five below a real change — `TestIconsEquivalent_ToleranceHeadroom` pins both populations). An icon either side cannot decode never compares equal, so a placeholder substitution surfaces rather than passing.

Three rules go with it. **Do not widen this to data blobs in general** — a certificate, a font and a VPN shared secret were probed in the same run and all three come back byte-identical, so a re-rendered certificate is real corruption and must keep failing. And **`data_files` is masked, for a reason one-sidedness does not cover.** It is Jamf Pro's integer handle for a stored icon, it exists only on the stored side, and the intersection compare would ignore it there. The strict comparator behind `ThreeWayCompare` and `PayloadsStructurallyEqual` would not: it demands matching keysets and values, and it compares *two server payloads* (the one stashed at apply against the one read back now) to tell an admin-side edit from a steady refresh. Two consecutive reads of one unchanged profile return **different** `data_files` values (wire-probed 2026-09-09), so before the mask every refresh of an icon-bearing profile reported drift nobody caused and the plan never settled. Mobile-device profiles emit the key; macOS profiles do not.

And **the tolerance applies only where one side of the comparison is a server response — never to two user-authored payloads.** The strict comparator is the delicate one here, because unlike `LenientEqualPlist` it straddles authoring layers: `structuralEqual` backs *both* arms of `ThreeWayCompare`, and arm one compares `planInput` against `lastInput`, two pieces of user HCL neither of which has ever been near Jamf Pro's renderer. So `payloadsStrictlyDiffer` takes an explicit `tolerateIconRerender` flag, threaded down through `structuralEqual`'s map and slice branches: `false` for the user arm, `true` for the drift arm (`lastCanonical` vs `serverNow`) and for `PayloadsStructurallyEqual`, whose operands are both server responses. Tolerating the user arm is silent data loss, not just lost fidelity: two authored icons differing by a small badge normalise inside the tolerance, so arm one reports no change, arm two agrees, `ThreeWayCompare` returns `DecisionNoOp`, and the plan modifier rewrites `plan.payloads` back to state — the edit can never be applied and no diagnostic is raised. The tolerance is still load-bearing in the drift arm even though its left side is an authored-shaped value, because `threeWayServerNow` falls back to `state.payloads` when private state has no server snapshot, and lenient self-healing leaves that in the authored form. `TestThreeWayCompare_AuthoredIconEditWithinToleranceApplies` pins the user arm; `TestThreeWayCompare_ServerIconRerenderIsNoOp` and `TestPayloadsStructurallyEqual_ServerIconRerender` pin the other two call sites.

If `inp_masked == srv_masked` the modifier suppresses the diff by setting `plan.payloads = state.payloads`. Otherwise the plan keeps the raw user input and Terraform plans the change.

**Trade-off accepted**: if the user authors a meaningful change to one of the masked keys (e.g. a hand-tuned `PayloadOrganization`), the provider will not detect drift — the server overwrites that value on the next write anyway, so a permanent diff would be the alternative. Document this in the `payloads` attribute `MarkdownDescription`.

**Update-path identifier injection.** On Update, the input builder parses `plan.payloads`, overwrites the top-level `PayloadUUID` and `PayloadIdentifier` with the values from `state.payloads`, and reserialises before PUT. Without this step every Update mints fresh UUIDs server-side and devices treat the update as a fresh profile installation ("ghost profile"). Top-level only is sufficient — nested `PayloadContent[i].PayloadUUID` survives PUT cycles without intervention once stored. Same mechanism as jamf-cli `profileconvert.InjectIdentifiers`.

**Asymmetric envelope `<level>` normalisation.** Classic accepts `<level>Computer</level>` on write but echoes `<level>System</level>` on read; `<level>Computer Level</level>` is silently rejected and defaults to `User`. Use the input-boundary translation pattern from §"Asymmetric server normalisation on type-style discriminator fields" — translate user-facing `Computer Level` / `User Level` to wire `Computer` / `User` on write, map wire `System` / `User` back on read.

**Read-back verification diagnostics.** When the mask compare fails after a write, the resource rolls the create back (update cannot) and fails with `payloadhelpers.PayloadFidelityErrorDetail`, which diffs supplied-vs-stored string leaves, skips masked keys, and classifies each divergence: line feeds/tabs deleted → recommend `&#13;` / `&#8232;` / an MCX payload; extra `&`/`<` entity layer → PI-827, unfixable client-side; non-BMP characters replaced or the enclosing dict dropped → Jamf-side limitation; value absent; unrecognised. Each finding names the plist path and quotes both forms `%q`-escaped around the first difference so invisible characters are visible. **Non-string leaves are diffed too** (`diffNonStringLeaves`), through the same intersection semantics, numeric leniency and icon equivalence the equality check uses, so the differ can never name a leaf the comparison was happy with — and, more to the point, can never come up empty on a tree that compared unequal. That was the second half of issue #418: the differ walked authored *string* leaves only, so a diverging data blob yielded zero findings while the trees were still unequal, and the operator got the unattributed fallback listing line feeds and ampersands that had nothing to do with it. A blob, a list or a dictionary is reported as a summary (`describeLeaf`) rather than dumped, and such a finding is marked `described` so the excerpting is skipped — excerpting a summary around its first differing character quotes half a sentence. Two constraints when editing it: bullet text must arrive **pre-wrapped** (Terraform re-wraps flush-left paragraphs but leaves indented lines as-is), and never add a static blanket explanation — a diagnostic that names characters the payload does not contain is worse than none. Acceptance `ExpectError` patterns for it must be **single tokens** (see §ExpectError line-wrap).

Reference implementation: the shared mask / compare / identifier-injection logic lives in `internal/common/payloadhelpers/` (`MaskPayload`, `PayloadsSemanticallyEqual`, `ThreeWayCompare`, `InjectTopLevelIdentifiers`); the per-resource glue is in `internal/resources/pro/macos_configuration_profile/` (`plan_modifiers.go` plan-time integration, `input_builders.go` / `crud.go` Update-path injection, `resource.go` MarkdownDescription disclosing the masked key set to users).

## Payload storage categories and the import fidelity gate

The read-back verification above is a **post-write** check, and it is only half a guard. Create can roll back when it fires; Update has nothing safe to roll back *to*, because the Classic API refuses to accept the original value back once the profile holds the mangled form (wire-verified: re-submitting a server's own unmodified `<payloads>` returns `409 Unable to update the database`). Repair is admin-UI-only.

That leaves **import** as the one way an unstorable profile can come under management: the provider will not create one, but it will happily adopt one authored in the admin UI. Such a profile imports clean and plans as a no-op, then the first apply that touches it — a rename, a scope tweak, anything — rewrites the payload irreversibly. `payloadhelpers.PayloadImportRisk` therefore refuses the import up front.

**Storage category is per `(platform, PayloadType)`, and the two platforms genuinely disagree.** Jamf Pro either re-renders a payload fragment (parses and re-serialises it — `&`, `<`, LF and TAB all survive, and unknown keys are preserved, so this is not schema filtering) or stores it verbatim (keeps the extra entity layer the Classic API requires on submission, and deletes LF and TAB; `>`, CR, U+2028/U+2029/U+0085 survive). Wire-probed 2026-08-06 against Jamf Pro 11.30.x:

| PayloadType | macOS | Mobile device |
|---|---|---|
| `com.apple.ManagedClient.preferences` | re-render | **verbatim** |
| `com.apple.applicationaccess` | **verbatim** | re-render |
| `com.apple.notificationsettings`, `com.apple.mobiledevice.passwordpolicy`, `com.apple.webcontent-filter` | re-render | re-render |
| `com.apple.systempolicy.control`, `com.apple.security.firewall`, `com.apple.SubmitDiagInfo`, `com.apple.servicemanagement`, `com.apple.extensiblesso` | re-render | not probed |
| `com.apple.shareddeviceconfiguration` | not probed | re-render |
| `com.apple.loginwindow`, `com.apple.MCX`, `com.apple.TCC.configuration-profile-policy`, `com.apple.vpn.managed`, `com.apple.wifi.managed`, `com.apple.dock`, `com.apple.screensaver`, `com.apple.SetupAssistant.managed`, `com.apple.systempreferences`, `com.apple.finder`, `com.apple.appstore`, `com.apple.dashboard`, `com.apple.system-extension-policy`, `com.apple.syspolicy.kernel-extension-policy` | verbatim | verbatim (where probed) |
| `com.apple.webClip.managed` | n/a | verbatim |

The first two rows are why a single global table is not an option — it would necessarily get one platform wrong. `testdata/reserved_character_corpus.mobileconfig` exists in both resources' fixtures with identical content and opposite expected verdicts; that pair is the regression guard.

**Rules for the table** (`faithfulPayloadTypes` in `importgate.go`):

- Only **re-render** types are listed. Everything absent — unprobed types, and every string slot outside the `PayloadContent` array such as a top-level `ConsentText` dictionary — is treated as verbatim.
- **Deny-by-default is deliberate and asymmetric.** A wrong "verbatim" guess costs one refused import, which is loud, immediate and recoverable. A wrong "re-render" guess lets a profile through to be corrupted irreversibly by its first update. Never guess in the second direction.
- **Add an entry only from a wire probe, never by inference.** The harness is `storage_category_probe_test.go` (build tag `payload_probe`). Inference is not idle caution: of the payload types with no prior classification, roughly a quarter turned out re-render (`servicemanagement`, `extensiblesso`, `webcontent-filter`), so deny-by-default alone would have refused them. `webcontent-filter` only yielded to a probe built from a real payload lifted off a tenant — every synthetic shape was schema-rejected, which is what `TestProbeStorageCategoriesFromRealPayload` is for.
- Note that a re-render *ingest* path may still add or drop keys (a Web Clip gains `Icon` / `data_files` and loses unrecognised keys). That is independent of the escaping question this table answers; do not conflate the two.

**Implementation constraints.**

- The gate builds a *predicted* stored form and diffs it through `diffPayloadTrees` — the same comparator, mask and classifier the post-write checks use. Do not write an independent predicate: sharing the comparator is what makes the gate and the post-write check unable to disagree, and reduces correctness to one testable function.
- **Gate on first-time hydration** — see [§First-time hydration detection in Read](write-semantics.md#first-time-hydration-detection-in-read) for the general rule. Both profile resources use the `state.General == nil` signal (`general` is schema-`Required`). An earlier revision got this wrong and every profile imported cleanly while every unit test passed.
- **Never gate ordinary refresh.** A profile already under management, or corrupted out-of-band, must keep refreshing so drift stays visible and the resource stays removable — otherwise the guard is worse than the trap.
- **No opt-out, by design.** A profile Jamf Pro cannot store faithfully cannot be managed safely, and a provider-global override would silently reinstate the corruption. A misclassification is fixed in the table for everyone; the diagnostic asks the operator to report the `PayloadType`.
- List resources **drop** a refused item from the stream and name it in one consolidated warning, rather than failing the stream and abandoning config generation for the rest of the tenant. Do not stream it identity-only with a nil `Resource`: the framework documents that as merely warning, but Terraform CLI 1.15 panics rendering it in `genconfig` (`cty.Value.GetAttr` on a null).

**Testing.** The predicate takes unit tests, but the *wiring* needs acceptance tests, and the fixture cannot be created through the provider — Create refuses it. The CI tenant keeps two permanent profiles named `Fidelity Test Sentinel [DO NOT DELETE]` (a macOS Login Window with `&` in `LoginwindowText`, and a mobile Web Clip with `&`/`<` in `Label` and `&` in `URL`), authored via admin-UI upload so they hold true `&`/`<` rather than the pre-mangled form. Acceptance tests resolve them **by name, never by ID**, and skip with a message describing the fixture when absent.

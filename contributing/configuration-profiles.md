# Configuration profile payloads

Payload diff suppression (mask-and-compare), storage categories and the import fidelity gate. Part of the [style guide](../STYLE_GUIDE.md).

The design, the wire behaviour behind it and the reasons for each tolerance are documented in the code: start at the package comment in `internal/common/payloadhelpers/doc.go`. This page holds the rules a change must obey.

## Configuration profile payload diff suppression (mask-and-compare)

Jamf Pro's classic configuration profile endpoints re-serialise the submitted `.mobileconfig`, so the stored payload never equals the authored one byte for byte. The provider does not predict those mutations. It runs `payloadhelpers.MaskPayload` over both sides and compares what is left.

- **Use the shared mask and comparators.** Plan-time suppression, read-back verification, drift detection and the import gate all go through `internal/common/payloadhelpers/`. Never write an independent comparator or predicate in a resource: sharing one is what stops those four checks from disagreeing about the same payload.
- **A new server normalisation goes into the mask**, on both sides, not into a per-`PayloadType` lookup of server defaults.
- **Masked keys are not drift-detected.** Jamf Pro overwrites them on every write. Each resource lists the masked keys in the `payloads` `MarkdownDescription`; update that list when the mask changes.
- **`PayloadContent` order is canonicalised, never compared.** Jamf Pro reorders the array on store. Do not pair entries by position or by `PayloadIdentifier`, which is masked.
- **Web clip icon tolerance applies only where one side is a server response.** Jamf Pro re-renders every web clip `Icon`, so that one leaf is compared as a picture. Between two authored payloads it must stay byte-exact, or an icon edit inside the tolerance is discarded without a diagnostic. `tolerateIconRerender` carries that distinction; keep it `false` for the user arm of `ThreeWayCompare`.
- **Do not widen the tolerance to other data blobs.** Certificates, fonts and VPN secrets come back byte-identical, so a difference there is real corruption and must keep failing.
- **Line breaks.** Jamf Pro deletes a literal line feed or tab from a string value in every payload type it stores verbatim and in every slot outside `PayloadContent`. A carriage return is the one whitespace character it keeps, it can be authored only as `&#13;`, and it reads back as a line feed. The mask therefore treats CR, CRLF and LF as equal, and the SDK sends `&#13;` references undecoded. Neither half works without the other. U+2028, U+2029 and U+0085 round-trip exactly and are not normalised.
- **Updates inject the stored top-level identifiers.** The input builder writes the stored `PayloadUUID` and `PayloadIdentifier` into the payload before the PUT (`InjectTopLevelIdentifierValues`). Without it Jamf Pro mints new ones and devices install the update as a new profile.
- **A failed read-back names the value.** Create rolls back; update cannot. The diagnostic comes from `payloadhelpers.PayloadFidelityErrorDetail`. When editing it, send bullet text already wrapped, since Terraform leaves indented lines as they are, and never add an explanation that is printed regardless of the findings.
- **`level` is translated at the input boundary.** The classic API accepts `Computer` and echoes `System`, and silently stores `Computer Level` as `User`. Follow [§Asymmetric server normalisation on `type`-style discriminator fields](schema-design.md#asymmetric-server-normalisation-on-type-style-discriminator-fields); the macOS mapping is in `macos_configuration_profile/mappings.go`.

The per-resource glue is in `internal/resources/pro/macos_configuration_profile/` and its mobile device sibling: `plan_modifiers.go` for plan-time suppression, `input_builders.go` and `crud.go` for identifier injection and read-back.

## Payload storage categories and the import fidelity gate

Some payloads cannot be written back unchanged: Jamf Pro stores most payload types verbatim, and in those an `&`, `<`, line feed or tab in a string is mangled on write. The provider refuses to create such a profile, so import is the one way it could come under management, and the first apply after that would corrupt it with no way to restore the original through the API. `payloadhelpers.PayloadImportRisk` refuses the import instead.

- **The storage category is per platform and `PayloadType`.** macOS and mobile device profiles disagree on some types, so there are two tables (`faithfulPayloadTypes` in `importgate.go`) and there must never be one.
- **Only probed re-render types are listed; everything else is refused.** Add an entry from a wire probe (`storage_category_probe_test.go`, build tag `payload_probe`), never by inference. A wrong refusal costs one import and a table entry. A wrong admission corrupts a profile irreversibly.
- **Gate on first-time hydration only**, using the `state.General == nil` signal ([§First-time hydration detection in Read](write-semantics.md#first-time-hydration-detection-in-read)). Never gate an ordinary refresh: a managed or externally damaged profile must keep refreshing so drift stays visible and the resource stays removable.
- **There is no opt-out.** A misclassified type is fixed in the table for everyone.
- **List resources drop a refused item** and name it in one consolidated warning (`ImportGateSkip`, `ImportGateSkipWarning`). Do not fail the stream, and do not stream the item with a nil `Resource`, which panics Terraform's config generation.

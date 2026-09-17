---
page_title: "Apple schema validation"
description: |-
  Blueprint payloads and declarations are checked against Apple's schemas during terraform plan, and every finding is an error.
---

# Apple schema validation

The provider carries Apple's own configuration profile and declarative device management schemas, generated from [apple/device-management](https://github.com/apple/device-management), and checks blueprint payloads against them during `terraform plan`.

**This is a breaking change.** A configuration that planned cleanly on `v0.32.0` and earlier can now fail its plan. The provider sends what it always sent, and rewrites no state. Only the plan outcome changes.

Two things moved:

- **Legacy configuration profile payload findings were warnings. They are now errors.** `component_blocks[].legacy_payloads` was checked before, but a finding that the embedded schemas could explain printed as a warning and the apply went ahead.
- **Declaration payloads are checked for the first time.** Both `component_blocks[].apple_declarations` and `component_blocks[].custom_declarations` now have their `payload` validated against the declaration type Apple publishes.
- **A custom settings payload sets exactly one preference domain.** A `com.apple.ManagedClient.preferences` payload that carries several is refused, and so is one that carries none. See [One preference domain per custom settings payload](#one-preference-domain-per-custom-settings-payload).

If your plan is clean, you have nothing to do.

## Spell keys exactly as Apple does

Case counts in a declaration, for both the key names and the declaration type. `AllowSiriAI` works. `allowsiriai` does not, and the plan names the spelling to use.

Legacy configuration profile payloads are more forgiving about case, so a spelling that carried in a profile is no evidence it will carry in a declaration. Check it against Apple's schema when you move one across.

## Authoring a declaration payload

`payload` is a JSON object string. Write it inline or read it from a file; both of these are equivalent:

```hcl
component_blocks = [
  {
    name = "Baseline declarations"
    apple_declarations = [
      {
        channel = "SYSTEM"
        type    = "com.apple.configuration.siri.settings"
        payload = jsonencode({
          Enabled              = true
          ForceProfanityFilter = true
        })
      },
      {
        channel = "SYSTEM"
        type    = "com.apple.configuration.passcode.settings"
        payload = file("${path.module}/declarations/passcode.settings.json")
      },
    ]
  },
]
```

The provider keeps the formatting and key order you wrote. Jamf Pro re-serialises a stored payload compact with its keys sorted, so the provider compares the two as JSON and keeps your bytes when they describe the same object. Without that, an indented file would be rewritten in state on the first read and diff on every plan after it. The comparison runs position by position, so it holds for a change Terraform itself makes. A declaration reordered directly in the Jamf Pro blueprint editor is compared against an unrelated prior payload, and its authored formatting is replaced by Jamf Pro's compact encoding, so the plan reads as a formatting change on top of the reorder it is correcting. The next apply settles it.

Use `file()` for a payload you did not write by hand. [DDM Explorer](https://apps.apple.com/gb/app/ddm-explorer/id6754861743) builds declarations and sends them to a test device: assemble one there, export its payload as JSON, and commit the file beside your configuration. You do not need `jsondecode`, because `payload` takes the file's text as it stands.

## Rewriting an `apple_declarations` block from v0.33.0

`apple_declarations` is the list of declarations. In `v0.33.0` it was an object holding a `declaration` list, so a configuration written against that release needs its brackets changed:

```hcl
# Before (v0.33.0)
apple_declarations = {
  declaration = [
    { channel = "SYSTEM", type = "…", payload = jsonencode({ … }) },
  ]
}

# After
apple_declarations = [
  { channel = "SYSTEM", type = "…", payload = jsonencode({ … }) },
]
```

State carries across on its own, through a state upgrader that runs on the first plan after the upgrade, so the configuration is the only thing to edit. Nothing else about the attribute changed, and the same declarations reach the same devices.

## The findings

| Finding | What it means | Can an old snapshot explain it? |
|---|---|---|
| Unknown Apple declaration type / unrecognised legacy payload type | The name is absent from the embedded schemas | Yes |
| Unknown key in payload | Apple declares no such key for this type | Yes |
| Unknown status item | A status subscription names an item Apple does not publish | Yes |
| Value not in enum | The value is outside the closed set Apple declares | Yes. Apple widens sets between revisions |
| Value out of range | A number falls outside Apple's declared bounds | Yes. Apple widens bounds between revisions |
| Missing required key | A key Apple marks required is absent | Yes. Apple relaxes `required` between revisions |
| Wrong value type | A string where a boolean is declared, and so on | No |
| Miscased type or key name | The name matches a declared one apart from case | No |
| Declaration kind does not match its type | `kind` disagrees with the declaration type's prefix | No |
| Custom settings payload sets more than one preference domain | One `com.apple.ManagedClient.preferences` payload carries several domains | No |
| Custom settings payload sets no preference domain | Its `PayloadContent` dictionary sets no domain, or sets only null ones | No |

The right-hand column is the one to read first. A finding marked **Yes** says so in its own detail, names the upstream branches and commits the schemas came from, and points at the escape hatch. A finding marked **No** is wrong against every revision of Apple's schema that declares the name at all, so the provider's age cannot be the cause.

The schemas are the union of Apple's `release` branch and its newest `seed_OS_*` (pre-release) branch, because Jamf Pro tracks seed: keys and declaration types show up in the blueprint editor while Apple still has them in pre-release. The diagnostic tells you when a finding comes from a pre-release part of the schema.

## A key the Jamf Pro editor offers, rejected by the plan

Upgrade the provider. The schemas **ship inside the provider release you have installed**, so there is no cache to clear and no setting to change. Every release carries a fresh copy, which keeps the gap between Apple publishing a key and a release delivering it short. It is not zero.

Until an upgrade is available, deliver the payload or declaration through `raw_component`, which the provider does not check.

## Delivering a declaration unchecked

`raw_component` passes your configuration to Jamf Pro as written, so nothing checks it. The keys below are Jamf's own names. Everywhere else the provider hands you attribute names matching the blueprint editor; here you write what Jamf stores.

Move the declaration into `raw_component` under the identifier the typed component would have used, and JSON-encode the configuration. Delete it from `apple_declarations` in the same edit. The provider rejects a block that sets both and reports `Component configured twice`.

```hcl
component_blocks = [
  {
    name = "App Settings"
    raw_component = [
      {
        identifier = "com.jamf.ddm-strict"
        configuration = {
          declarations = jsonencode([
            {
              channelType = "SYSTEM"
              kind        = "CONFIGURATION"
              type        = "com.apple.configuration.app.settings"
              payloadKey  = 1
              payload = {
                Allowed = { DeniedApps = ["com.apple.screenshots"] }
              }
            },
          ])
        }
      },
    ]
  },
]
```

Set `payloadKey` yourself. It is the 1-based position of the declaration within the request, and it is what a `$PAYLOAD_<n>` cross-reference from another declaration resolves against. The typed components derive it from list order; `raw_component` derives nothing, so a declaration without a key cannot be referenced.

## One preference domain per custom settings payload

A `com.apple.ManagedClient.preferences` payload, which the Jamf Pro profile editor calls "Application & Custom Settings", sets **exactly one** preference domain under `PayloadContent`. Any other count is refused during `plan`.

**This is a breaking change.** A configuration that planned cleanly on `v0.32.0` and earlier can now fail. A payload that named three domains only ever delivered one of them.

A Mac applies one domain of several and drops the rest. Nothing reports the loss. The apply succeeds, Jamf stores every domain, the deploy reports `SUCCEEDED`, `com.apple.ManagedClient` logs nothing, and every later plan settles. Which domain survives is not predictable: of two profiles probed on macOS 26.6, one kept the second of three domains and the other kept the last of four. Split the same domains one per payload and all of them apply, which is what the Jamf Pro editor produces.

Write one payload per domain. A component block may carry as many custom settings payloads as it has domains. A repeated `payload_type` used to be rejected outright, and for this one payload type it no longer is.

Before:

```hcl
legacy_payloads = [
  {
    payload_type = "com.apple.ManagedClient.preferences"
    settings = jsonencode({
      PayloadContent = {
        "com.apple.Safari"         = { Forced = [{ mcx_preference_settings = { AutoOpenSafeDownloads = false } }] }
        "com.apple.SoftwareUpdate" = { Forced = [{ mcx_preference_settings = { AutomaticCheckEnabled = true } }] }
      }
    })
  },
]
```

After:

```hcl
legacy_payloads = [
  {
    payload_type = "com.apple.ManagedClient.preferences"
    settings = jsonencode({
      PayloadContent = {
        "com.apple.Safari" = { Forced = [{ mcx_preference_settings = { AutoOpenSafeDownloads = false } }] }
      }
    })
  },
  {
    payload_type = "com.apple.ManagedClient.preferences"
    settings = jsonencode({
      PayloadContent = {
        "com.apple.SoftwareUpdate" = { Forced = [{ mcx_preference_settings = { AutomaticCheckEnabled = true } }] }
      }
    })
  },
]
```

Splitting a payload reissues the payload identifiers Jamf owns for that block, so the profile reinstalls once and every domain then applies.

**A domain set to `null` counts as none.** The platform discards a null-valued key before storing the payload, so a payload whose only domain is null stores nothing at all. An empty `PayloadContent` dictionary goes the same way, which is why both carry their own diagnostic: the settings you wrote cannot be read back, and the apply fails with `Provider produced inconsistent result after apply`. Write a null domain alongside a real one and the real one still counts.

**Split the domains rather than reaching for `raw_component`.** Moving the block silences the check and changes nothing a device sees. The payload still carries several domains, a Mac still drops all but one, and the block's payload identifiers are reissued on every write. The escape below answers a schema finding, where the provider's embedded key tables may be older than your tenant. This one is device behaviour, and a newer table will not change it.

## Delivering a legacy configuration profile payload unchecked

**Move the whole block's payloads, not the one that failed.** Every `legacy_payloads` entry in a component block folds into a single `com.jamf.ddm-configuration-profile` component whose `payloadContent` is the array of payloads. The provider rejects a partial move with `Component configured twice` and writes nothing, so move every payload in the block or leave them where they are.

Before:

```hcl
component_blocks = [
  {
    name = "Safari Restrictions"
    legacy_payloads = [
      {
        payload_type = "com.apple.applicationaccess"
        settings = jsonencode({
          allowSafariHistoryClearing = false
          allowSafariPrivateBrowsing = false
        })
      },
      {
        payload_type = "com.apple.dock"
        settings     = jsonencode({ tilesize = 48 })
      },
    ]
  },
]
```

After:

```hcl
component_blocks = [
  {
    name = "Safari Restrictions"
    raw_component = [
      {
        identifier = "com.jamf.ddm-configuration-profile"
        configuration = {
          payloadDisplayName = "Safari Restrictions"
          payloadContent = jsonencode([
            {
              payloadType = "com.apple.applicationaccess"

              allowSafariHistoryClearing = false
              allowSafariPrivateBrowsing = false
            },
            {
              payloadType = "com.apple.dock"

              tilesize = 48
            },
          ])
        }
      },
    ]
  },
]
```

Two things to carry across:

- **Each payload's settings sit alongside `payloadType`**, not nested under a `settings` key. The typed attribute merges them in.
- **`payloadDisplayName` is per component.** The typed attribute uses the blueprint's own name.

Moving a block to `raw_component` shows up in the plan as one component destroyed and another created. Read it before you apply.

## Further reading

| To look up | Where |
|---|---|
| Building a declaration payload against a test device | [DDM Explorer](https://apps.apple.com/gb/app/ddm-explorer/id6754861743) |
| Every declaration type and key the provider checks | [Apple's declarative device management schemas](https://github.com/apple/device-management/tree/release/declarative) |
| Every payload type and key the provider checks | [Apple's configuration profile schemas](https://github.com/apple/device-management/tree/release/mdm/profiles) |
| Blueprints themselves | [Blueprints Guide](https://learn.jamf.com/r/en-US/Jamf-Blueprints-Guide) |

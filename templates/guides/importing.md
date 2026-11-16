---
page_title: "Importing existing objects"
description: |-
  Why the first plan after an import proposes removing blocks you never wrote, and what applying it does.
---

# Importing existing objects

Import an object Jamf Pro already holds and the first `terraform plan` afterwards often proposes
removing blocks your configuration never mentions:

```
  # jamfplatform_pro_mac_app_store_app.excel will be updated in-place
  # (imported from "2")
  ~ resource "jamfplatform_pro_mac_app_store_app" "excel" {
      - self_service = {
          - install_button_text  = "Install" -> null
          - notification_method  = "Self Service" -> null
        } -> null
      - vpp          = {
          - assign_vpp_device_based_licenses = true -> null
          - total_vpp_licenses               = 5000 -> null
        } -> null
    }

Plan: 1 to import, 0 to add, 1 to change, 0 to destroy.
```

Nothing has gone wrong. On nearly every resource, applying that plan touches the state file and
nothing else: Jamf Pro keeps the values you see above. On [a few](#where-the-removal-is-real) it
wipes them. This guide covers both cases and how to settle the plan in each.

## Import records everything Jamf Pro reports

Import reads the whole object and records every optional block Jamf Pro reports, whether or not
your configuration mentions it. An app you imported against a five-line `general` block arrives
with its scope, its Self Service settings and its volume purchasing details alongside.

That is the point of importing. A block left out of state would go unmanaged and unseen, and no
later plan would tell you it was missing.

So state can hold more than your configuration does.

## Terraform then plans towards your configuration

Terraform compares the two and offers to close the gap. Where state holds a block your
configuration does not, Terraform plans to remove it and renders `-> null` on every attribute
inside. That plan describes the state file. It tells you nothing about the object's health.

The same plan appears when you delete a block from a configuration you have been applying for
months. Import is the most common way to meet it, because import is the one operation that fills
state without your having written anything.

## What applying it does

What you do not declare, Terraform does not manage. It leaves an undeclared block out of the
request, Jamf Pro keeps whatever it holds, and the state file is the only thing that changes. The
plan cannot show you that, because a plan renders state.

`jamfplatform_pro_mac_app_store_app`, `jamfplatform_pro_policy` and
`jamfplatform_pro_restricted_software` each carry an acceptance test that drops a block, applies,
and then reads the object back from Jamf Pro. Every value the block held is still there.

This is what lets you split management of one object. Declare the scope and let the help desk own
the Self Service description. Declare a policy's packages and leave its icon to whoever chose it.
Anything you leave out is yours to change in the admin UI, and later plans stay quiet about it.
Anything you declare, Terraform owns: change it in the admin UI and the next plan reports drift.

Dropping a whole block is not always the same as dropping a field inside one. On several resources
Terraform writes an omitted field empty when you declare the block around it, which clears the
value, while omitting the block leaves everything in it alone. Each attribute's own description
says which applies, so check the field before you trust the block-level answer.

## Where the removal is real

!> **Two resources reset what you leave out instead of keeping it.** The plan reads identically to
the harmless one above. The outcome does not.

| Resource | What a proposed removal really removes |
|---|---|
| `jamfplatform_pro_app_installer` | Terraform writes the notification and Self Service settings whole, so Jamf Pro resets both blocks to their defaults. The description, the deadline and the notification messages go with them. |
| `jamfplatform_security_cloud_uem_connect` | Terraform writes the sync settings whole, so Jamf Security Cloud resets `user_data_field_mapping` and `group_membership_mapping` to their defaults. |

On these two, declare what you want to keep before you apply. Each says so in its own Import
section too.

## Seeing what an import will hold

Write the import block, run `terraform plan`, and read it before you write any more configuration.
The plan lists every value Jamf Pro holds for the object. Copy across what you want Terraform to
manage and leave the rest out.

`terraform plan -generate-config-out=<file>` writes a starting configuration for you. It declares
everything, so trim it rather than applying it as generated.

A whole tenant is a bigger job than one object.
[Jamformer](https://concepts.jamf.com/en/concepts/jamformer/) does that job: the CLI discovers what
your tenant holds across more than 100 resource types, writes the import blocks, generates the
configuration, and rewrites cross-resource references for you. Read and trim what it hands you
before you apply any of it. It is a first draft.

Everything on this page applies to that draft. Jamformer declares the optional blocks it found, so
trimming one is the same decision as leaving it out by hand.

## Settling the plan

Two ways, and which one is right depends on the resource.

Declare what you want to keep. Copy the values out of the plan output into your configuration, then
re-plan. The removal disappears, because state and configuration now agree, and Terraform manages
those blocks from then on. **This is the only safe route on the two resources above.**

Or apply once and let state converge. The removal applies, state drops the blocks, and every later
plan is clean. Nothing in Jamf Pro moves. Choose this where you want the object co-managed, and
only on a resource that preserves an undeclared block.

Either way it is a one-time cost. A later read does not put the blocks back, so the plan settles
once and stays settled.

## Certificate blocks import differently

A block holding a certificate is its own case, because Jamf Pro reports no readable copy of a
keystore, a password, or the version counter that triggers a re-send. Each certificate-bearing
resource documents what its own import restores, and what the first apply afterwards sends, in its
Import section. Read that section before importing one.

Two resources never fill a certificate block on import. Both are deliberate.

`jamfplatform_pro_sso_settings` leaves `signing_certificate` unset even when your tenant holds a
certificate. Declaring that block is how you hand the certificate to Terraform to manage. If import
handed it over for you, later dropping the block would delete your tenant's signing certificate.
Omit the block and Jamf Pro keeps the stored certificate.

`jamfplatform_pro_user_initiated_enrollment_settings` leaves `mdm_signing_certificate` and
`developer_certificate` unset because nothing survives to restore them from. Jamf Pro reports
neither the keystore filename nor the rotation version for a stored signing identity. Omit a block
and the stored certificate stays put. To remove one, turn off the toggle that governs it.

## Configuration profiles that are refused

`jamfplatform_pro_macos_configuration_profile` and
`jamfplatform_pro_mobile_device_configuration_profile` refuse to import a profile whose payload
Jamf Pro would not store back unchanged. The error is "This profile cannot be managed by
Terraform". It names the profile and up to three of the values at fault, showing each as it is in
Jamf Pro now and as it would be after a Terraform write. Nothing is imported and nothing in Jamf
Pro changes.

The cause is an `&`, a `<`, a line feed or a tab in a text value of the payload. A profile built in
the Jamf Pro admin UI can hold those characters, but for most payload types a write through the
API stores `&` as `&amp;` and `<` as `&lt;`, and deletes line feeds and tabs. If the import went
ahead, the first apply that touched the profile, even to rename it, would rewrite the payload that
way, and Jamf Pro would then refuse the original value back. The repair could only be made in the
admin UI.

There is no setting that overrides the refusal. You can:

- Remove the characters the error names in the Jamf Pro admin UI, then import again.
- Leave the profile out of Terraform and read it with the data source of the same name.

The check applies to `terraform import` and to an `import` block alike. When you generate
configuration for many profiles at once with `terraform query`, a profile that would be refused is
left out and one warning lists the profiles that were skipped, twenty at most. The rest are
generated as normal. Import a skipped profile on its own to see which value is at fault.

A profile Terraform already manages is never refused. It keeps refreshing, so you can still see
drift in it and still remove it.

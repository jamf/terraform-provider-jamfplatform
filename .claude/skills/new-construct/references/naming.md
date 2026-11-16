# The UI-naming review

Authoritative: `STYLE_GUIDE.md` §Attribute names mirror the Jamf Pro admin UI when the wire
name is cryptic, §Translating UI labels/presets to wire values, §User-facing descriptions are
UI-aligned. Run this review **before the schema exists**. Renames are free while the provider
is unshipped and breaking after merge, so late is the expensive direction.

## The rule, including the half that gets missed

Rename to the UI label when the wire name is cryptic **OR differs materially from it**. The OR
is the part that gets dropped: `datacenter` is not cryptic, but the UI says "Egress region"
and never "datacenter" — so it renames.

**Enum values follow the UI too**, not just attribute names: `"AES-256"` → `aes256`,
`"Group 14 (modp2048)"` → `modp2048`, `"First available"` → `ACTIVE_STANDBY`,
`"30 minutes"` → `1800`. Be consistent across a resource — translating one enum and leaving
its neighbour wire-spelled reads badly in a single config. Skip the table only when the UI and
wire strings are identical.

## What stays wire-spelled

A UX phrase is not a field name. Keep the wire-ish name when the UI label describes a
*relationship* rather than naming the value: "Reachable via" → `gateway_id`, "Choose your
gateways" → `gateway_ids`. `reachable_via = "a7d2"` says nothing. Keep the **ID** rather than
the name wherever a sibling resource reference has to work.

## Where the mapping lives

`mappings.go` — "Lookup tables and name mappings" in the file-conventions table. There is no
`enums.go` in that table; do not invent one.

The wire mapping goes in the **package doc comment as a table**, not in comments beside each
attribute: a schema is built inside a function body, and STYLE_GUIDE §Go Conventions forbids comments there.

## Keeping a label table honest

A round-trip test passes by construction even with a wrong pairing, so use three guards
instead:

1. Key the map on the SDK's generated value **constants** — a renamed value breaks the build.
2. Derive the accepted-label slice from the SDK's `*Values()` helper — a coverage test then
   catches a value with no label, and a label outliving its value.
3. Wire-probe every stored value.

Flag any label you did not read off an actual screen. A wrong label is a docs bug, not a
data-integrity one — but say which you are unsure of.

## Scope of the review

Run it across **every construct in the PR**, not just the one asked about. The pass that fixed
the ZTNA gateway found the same class of miss in the DNS zone (`name_servers` →
`authoritative_name_servers`, `ip` → `ip_address`) and, separately, an IPv4 validator
duplicated across both packages — extracting it per the 2-consumer rule exposed a real bug the
duplication had been hiding.

## After a rename, the sweep

A grep for the old attribute name misses: the *key* of a `TestCheckTypeSetElemNestedAttrs`
map (`map[string]string{"ip": …}`), dotted state paths (`"…name_servers.0.ip"`), and HCL
arguments whose alignment padding differs (`datacenter         = %q` when you matched
`datacenter = %q`). Grep the old **value** literals too — a rename that changed the vocabulary
invalidates assertions on values, not just names (`"1800"` → `"30 minutes"`).

---
page_title: "Impact alerts"
description: |-
  Show how many computers and mobile devices a change reaches, during terraform plan.
---

# Impact alerts

Save a policy, profile, app or group in Jamf Pro and it shows an **impact alert**: how many devices the change affects. Set `impact_alerts` and the same signal appears during `terraform plan`. The provider adds one Jamf Pro does not show, for [policy dependencies](#policy-dependency-changes) such as scripts and packages:

```hcl
provider "jamfplatform" {
  impact_alerts = true
}
```

```
Warning: Impact alert — this policy will be scoped to 47 of 312 computers (15%)

  with jamfplatform_pro_policy.security_baseline,
  on main.tf line 12, in resource "jamfplatform_pro_policy" "security_baseline":
  12:   scope = {

Counted from 2 groups: All Managed Clients (44), Lab Macs (5).
Less 1 group excluded: Contractors (2).

Group membership is a snapshot taken during this plan and can change before
or during apply.
```

Off by default; also settable with `JAMFPLATFORM_IMPACT_ALERTS`. Alerts are advisory. They never block a plan, and a tenant that cannot be read produces a single notice.

## What triggers an alert

Any change to an object deployed to devices, or to an object that scope is based on. Objects with no planned change stay silent.

A scope change reports what moves:

```
This change is adding 12 computers and removing 3 computers.
```

A payload change reports the audience it reaches. Adding a script to a policy affects every computer already in its scope:

```
Counted from 2 groups: All Managed Clients (44), Lab Macs (5).
The scope is unchanged; these computers will receive the updated policy.
```

Creating an object reports what will start receiving it; deleting one reports what stops.

## Policy dependency changes

A script, package, printer, Dock item, directory binding or disk encryption configuration has no scope of its own. But every policy referencing it delivers it to that policy's audience. Jamf Pro shows no alert for that. The provider does:

```text
Impact alert — this script affects up to 3 of 4 computers (75%), via 56 policies

Used by 56 enabled policies: Jamf Auto Update - Adobe Acrobat DC - Ongoing, ... and 48 more.
Also 9 disabled policies, delivering nothing until enabled: ... and 1 more.
Counted from their combined scope: 36 groups: All Managed (3), All Laptops (2), ... and 31 more.
Computers in more than one are counted once.

Searched 295 policies. Policy usage and group membership are a plan-time snapshot and can change before apply.
```

How the figure is built:

- Computers count once. Policy scopes overlap, so the total is the union of the affected audiences, not the sum.
- Disabled policies are listed, never counted. They deliver nothing until enabled.
- A policy that could not be read makes the figure a lower bound. It may add audience but cannot take any away, so the figure reads `or more` and names the unread policies among the caveats. Combined with a dropped exclusion, which pulls the other way, the figure reads `an estimated` instead.
- Exclusions combine only when every policy shares them. An exclusion belongs to its declaring policy, so a computer excluded from one but targeted by another still receives the object. An exclusion carried by every contributing policy is subtracted, because a computer all of them exclude receives the object from none. Any other becomes a narrowing caveat and the figure reads `up to`.

Creating one of these raises no alert: nothing can reference an id the tenant has not issued.

`terraform apply -replace` on one of them raises no dependency alert either, when the configuration is otherwise unchanged. The replacement gets a new id, so every policy that referenced the old one loses the dependency. Check the references by hand before replacing a script, package, printer, Dock item, directory binding or disk encryption configuration.

## Reading the figure

The wording states how the number relates to reality. Jamf Pro's scope model narrows as well as broadens, so an input that cannot be evaluated does not always mean the true figure is higher.

| Wording | Meaning |
|---|---|
| `47 of 312 computers` | Every input was countable. |
| `up to 47 of 312 computers` | Something narrows the audience by an amount not determinable during a plan. |
| `47 or more of 312 computers` | Something adds devices that could not be counted. |
| `at least 47 of 312 computers` | As `47 or more`. The qualifier leads when the figure comes from a resource that can span both estates. |
| `an estimated 47 of 312 computers` | Inputs pull both ways, so the figure bounds it from neither side. |
| `47 computers`, no proportion | The scope names unmanaged devices, so a share of the managed estate would mislead. |

Anything unevaluable is named, with the direction it moves the figure:

```
Not resolvable during plan; the true figure may be lower:
  · limitations.network_segment_ids (1) — network segments are matched against a device's network location when it checks in
```

Network segments and iBeacon regions are matched against where a device is when it checks in. User-based targets reach devices through user assignment, which Jamf Pro resolves.

Scope a resource to a group this same plan creates and no figure is reported: a new group has no membership until applied, and a smart group's membership comes from its criteria, not your configuration.

Resources targeting both estates report each side with its own denominator, because three Macs and three iPads are not the same change:

```
Impact alert — this ebook will be scoped to 3 of 4 computers and 0 of 1 mobile devices
```

## Group and class changes

Editing a group reports the knock-on effect:

```
Warning: Impact alert — this static computer group changes from 4 computers to 7 computers

Every object scoped to this static computer group applies to whatever joins it,
and stops applying to whatever leaves.
This change adds 3 computers.
```

A smart group reports its present membership and states that the outcome is decided after apply, when Jamf Pro re-evaluates the criteria against inventory.

Editing a group and something scoped to it in one plan produces two alerts, one from each side.

## Using alerts in CI

Read them from `terraform plan -json`, which carries every alert:

```bash
terraform plan -json \
  | jq -r 'select(.["@level"] == "warn")
           | .diagnostic
           | select(.summary | startswith("Impact alert"))
           | [.address, .summary] | @tsv'
```

To gate on the number, match the verb, then the figure. `scoped to` for a create, `affects` for an update or delete:

```
(scoped to|affects) (up to |at least |an estimated )?([0-9]+)( of [0-9]+)?
(scoped to|affects) ([0-9]+) or more( of [0-9]+)?
```

The `of` clause is optional, because a figure with no proportion drops it. An alert spanning both estates joins two `X of Y` clauses with ` and `, as in `3 of 4 computers and 0 of 1 mobile devices`, so match `([0-9]+) of ([0-9]+)` globally to read the second estate. Group and class alerts phrase membership instead (`will contain 7 computers`, `changes from 4 computers to 7 computers`). Gate those on the arithmetic in the detail, such as `This change adds 3 computers.`

Policy dependency alerts use the same `affects` figure, then append the policy count. Their counts sit in fixed phrases:

| Pattern | Reads |
|---|---|
| `via ([0-9]+) (policy\|policies)$` | Enabled policies carrying the change (end of summary) |
| `(?m)^Used by ([0-9]+) enabled (policy\|policies):` | Same count, in the detail, followed by names |
| `(?m)^Also ([0-9]+) disabled (policy\|policies), delivering nothing until enabled:` | Referencing policies contributing no devices |
| `(?m)^Searched ([0-9]+) (policy\|policies)` | Policies read to build the figure |
| `used only by disabled policies$` | Every referencing policy is disabled, so nothing is delivered yet. Latent, not safe |
| `no policy uses this` | Nothing references the object; carries no figure to gate on |

The last two summaries carry no `affects` figure at all, so a pipeline gating only on the numeric patterns above skips them silently. Match them explicitly if you want to see a change that reaches nothing, or reaches only disabled policies. `no policy uses this` deliberately does not match the partial-sweep wording (`no policy found using this …, but the search was incomplete`): an incomplete search is not a finding of no usage, and the two should not be treated alike.

The summary's trailing `via N policies` adds a number after the figure. Anchor the device count on the `of` clause; do not take the last integer in the line.

**Anchor the detail patterns to a line start, as written above.** Each detail sentence begins its own line. The only free text in the detail is policy names, and those always appear mid-line, inside the `Used by` and `Also` lists. Names come from the tenant, so whoever can create or rename a policy chooses that text. A policy named `Searched 0 policies.` would satisfy an unanchored `Searched ([0-9]+)` before the genuine sentence does, and a pipeline taking the first match would read the attacker's number. The `(?m)^` prefix removes that reach: a name can never start a line. Control characters in names are stripped before rendering, so a name cannot manufacture a line start either.

The `Searched` count is always the number of policies actually read. If any could not be read, the sentence continues instead of being reworded: `Searched 294 policies of 295 (1 could not be read, so this answer may be incomplete).` Gate on the phrase without the full stop. A partial sweep changes two other things. The figure takes the `or more` form, so match the second pattern above as well as the first, and a summary reporting no usage says the search was incomplete instead of denying it outright.

## Permissions

Impact alerts read tenant data with the credential that runs the plan. No resource's **Required Jamf permissions** table lists these reads. A missing permission never fails the plan; the alert falls back to a single "impact alert unavailable" notice. For full figures, grant the integration Read on each of these in Jamf Account:

| Category | Permission | Needed for |
|---|---|---|
| Deployment | Policies | Policy dependency alerts (the tenant-wide policy sweep) and policy scope |
| Inventory | Device groups | Group and class membership |
| Inventory | Devices | Computer and mobile device counts |
| Organizational context | Buildings | Building-scoped targets |
| Organizational context | Departments | Department-scoped targets |

## Cost

Reads are cached for the lifetime of the plan, so two resources naming the same group read it once. A plan that changes nothing reads nothing.

| Read | When |
|---|---|
| Group list and device totals | Once per plan, on the first resource needing them |
| One group's membership | Per group named by a changing scope |
| One inventory query | Per distinct set of named devices, buildings or departments |
| Building and department names | Once per plan, if a mobile device scope names one |
| Every policy in the tenant | Once per configured provider instance, only if the plan changes a policy dependency |

The group list is paginated at 100 groups per request. Membership is one read per distinct group named by anything changing in the plan, including groups only entering or leaving a scope.

The policy sweep is the largest single cost, which is why it is lazy. Jamf Pro has no endpoint answering "which policies use this script". The Classic API's subset endpoint cannot trim the payload either: it silently omits the `PackageConfiguration` section package references live in. So every policy is read in full. Measured at ~6 seconds and ~1MB for 295 policies, so expect roughly a minute at 3,000. A plan changing no dependency never triggers it.

The sweep is not the whole cost of a dependency alert. Once the using policies are known, their combined scope is resolved like any other, so a dependency used by many policies also pays one membership read per distinct group that scope names. Dozens, for a widely-used script. Those reads are cached for the plan, so a second dependency naming the same groups adds nothing.

Read "once per configured provider instance" carefully. It is not once per plan: the cache is built per provider configuration, so two `provider` blocks aliased against the same tenant each sweep it.

The sweep needs a credential that can read every policy in the tenant. An API role without policy read does not fail the plan. The alert degrades to a single "impact alert unavailable" notice, and no dependency figure is produced.

Two controls bound this, and they are easy to confuse. `min_request_interval_ms` gates the time between request *starts* across all traffic. That makes it a throughput ceiling on the whole plan, not a concurrency limit: at 100ms no plan exceeds 10 requests per second, however many resources Terraform evaluates in parallel. It defaults to 0. The sweep separately caps its own parallelism at 5 concurrent reads, matching the [API scalability guidance](https://developer.jamf.com/jamf-pro/docs/jamf-pro-api-scalability-best-practices) and the point where measured throughput stops improving. Trade plan speed for a gentler load profile by raising `min_request_interval_ms`.

## Scope of reporting

Every figure is a snapshot: group membership is re-evaluated continuously, so treat the number as what would happen if the change applied at that moment.

`jamfplatform_pro_vpp_assignment` and `jamfplatform_pro_vpp_invitation` raise no alert. Their scope targets Jamf Pro users and user groups, with no device category to count.

The policy sweep reads policies only. Patch Management is a second delivery channel for packages: a patch software title assigns a package per software version, and a patch policy carries its own scope. It is not searched, so a package delivered only that way is reported as used by no policy. The alert for a package says so, so it does not read as an all-clear.

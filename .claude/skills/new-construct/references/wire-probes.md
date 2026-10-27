# Wire probes

Wire evidence drives schema decisions, so collect it **before** sketching the schema. Raw
bodies go under `local-testing/<endpoint>/` (gitignored). Never paste real device UDIDs,
serials, tenant names or directory names into a committed file.

## Reaching the gateway directly

Scope travels in a **header**, not the URL path (SDK v0.17.0 onward). Probe recipes written
before that — including every kickoff prompt in `.claude/prompts/` — put
`/tenant/<tenant-id>/` in the path and are wrong now.

```sh
TOK=$(jamf-cli platform auth token --field token -p <profile>)   # opaque, not a JWT
jamf-cli config list                                            # region + tenant-id per profile

# Pro: version segment in the path, scope in the header
curl -s -H "Authorization: Bearer $TOK" -H "X-Tenant-Id: <tenant-id>" \
  "https://<region>.api.jamfcloud.com/pro/v1/<endpoint>"

# ProClassic: no version segment, no JSSResource segment, and ask for JSON
curl -s -H "Authorization: Bearer $TOK" -H "X-Tenant-Id: <tenant-id>" \
  -H "Accept: application/json" \
  "https://<region>.api.jamfcloud.com/proclassic/<classic-resource>"

# Security Cloud: one unified prefix for all five namespaces
curl -s -H "Authorization: Bearer $TOK" -H "X-Tenant-Id: <tenant-id>" \
  "https://<region>.api.jamfcloud.com/securitycloud/<endpoint>"
```

`X-Environment-Id` substitutes for `X-Tenant-Id` under an environment-scoped integration —
sending the wrong one is `403 OWNERSHIP_FORBIDDEN` even when both IDs belong to the same
customer. Capture the real URL of any CLI command with `-vvv`. Authoritative prefix logic
lives in the SDK's `internal/client`.

Two `jamf-cli` traps: a classic update through the CLI is read-modify-write, so use raw curl
when probing what a bare PUT does; and `jamf-cli delete` needs `--yes` or it hangs waiting on
a prompt that never renders.

## The probe list worth writing down

At minimum, per endpoint:

| Probe | Answers |
|---|---|
| `GET` at factory defaults | are sub-objects present-but-null, or absent? decides Read normalisation |
| `GET` minimal vs populated | which fields the server omits vs nulls |
| `POST` create request + response | fields the create drops (a POST-then-PUT dance may be needed) |
| `PUT` partial vs full body | full-replace or merge; what an omitted field does |
| `PUT` with `{}` / `{"block": {}}` | does empty clear, get rejected, or preserve? |
| collection `[]` vs omitted vs populated | the three-way clear semantics — **per resource, never inherited** |
| an out-of-vocabulary enum value | is it enum-constrained or freeform? |
| a value outside the UI's presets | `OneOf` validator or range validator? |
| unit-bearing fields | the wire multiplier behind a UI number+unit control |
| create the same object twice: same name, upper case, trailing space, `é`, leading space | whether the service refuses a duplicate, which key it compares, and how (case, spaces, accents). Never assume the key is the name: packages compare `fileName`, DNS zones `domains`, inventory preload `serialNumber`. Probe the write API the resource uses, since one pool can compare differently per API |

## Honesty rules

- A probe you did not run is marked **NOT-PROBED with the reason**. Never write a
  plausible-looking result you inferred. In a pre-spike doc, every entry is a *probe to run*,
  not a finding — label it that way so the next reader cannot mistake one for the other.
- **Probes mutate.** Anything destructive runs against disposable test devices or a
  non-production tenant, or it is marked NOT-PROBED.
- Absence of tenant data is not evidence. If the tenant has no object of the needed kind, the
  test skips — it does not infer.
- Respect the load budget: max 5 concurrent requests, ~10 req/s.

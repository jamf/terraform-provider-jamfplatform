# Acceptance test fixtures

`sample.mobileprovision` is an **unsigned** sample provisioning profile (a plain
XML property list, no CMS signature) used to exercise the create-with-blob path.
It carries `UUID` `00000000-0000-0000-0000-000000000000`, `Name` and `TeamName`
`Example`, and an `ExpirationDate` of **9999-03-05**, so it does not expire.

Provenance: copied verbatim from
[facebook/buck](https://github.com/facebook/buck/blob/b3fc2598badeb689bed543f306f2b4867dbcd068/test/com/facebook/buck/apple/testdata/provisioning_profiles/sample.mobileprovision)
at commit `b3fc2598badeb689bed543f306f2b4867dbcd068`, licensed under the Apache
License 2.0. It contains no certificates, devices or account data.

Jamf Pro accepts it: wire-verified 2026-10-20 on Jamf Pro 11.32.0, a classic
`POST /JSSResource/mobiledeviceprovisioningprofiles/id/0` carrying it answers 201
and the stored record reports the embedded UUID. This replaces the earlier
Apple-signed enterprise fixture, which expired on 2026-06-26 and whose README
recorded that Jamf rejects unsigned blobs with HTTP 500; that does not hold on
11.32.0. If a later Jamf Pro release starts validating the signature, the create
steps fail with 500 and a signed profile is needed again.

The acceptance test base64-encodes the file at runtime and feeds it to
`profile_data`; the tests assert the computed fields are populated, not their
exact values.

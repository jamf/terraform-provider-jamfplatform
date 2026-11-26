---
page_title: "Package files from cloud storage"
description: |-
  Read a jamfplatform_pro_package file from Amazon S3, Azure Blob Storage or an https server that needs a credential, with change detection.
---

# Package files from cloud storage

`package_file_source` on `jamfplatform_pro_package` reads the package file from four kinds of place:

| Source | Example | Change shows in the plan |
|---|---|---|
| Local path | `/builds/MyApp-1.0.0.pkg` | Yes |
| Amazon S3 | `s3://my-packages/releases/MyApp-1.0.0.pkg` | Yes |
| Azure Blob Storage | `azblob://mystorageaccount/packages/MyApp-1.0.0.pkg` | Yes |
| URL | `https://artifacts.example.com/MyApp-1.0.0.pkg` | No |

This guide covers the two cloud stores and `https` servers that need a credential. A plain URL needs no setup.

```hcl
resource "jamfplatform_pro_package" "myapp" {
  display_name        = "MyApp 1.0.0"
  file_name           = "MyApp-1.0.0.pkg"
  package_file_source = "s3://my-packages/releases/MyApp-1.0.0.pkg"
}
```

With no further configuration, that reads the object with the AWS credentials the machine already has. An `azblob://` source does the same with the machine's Azure credentials.

## Only `package_file_source`

No other attribute reads `s3://` or `azblob://`, and the provider's `file_sources` block applies to nothing else. `manifest_file_source`, `icon_file_source` on `jamfplatform_pro_icon`, `image_file_source` on `jamfplatform_pro_self_service_branding_image` and `icon_source` on `jamfplatform_pro_enrollment_customization` take a local path or an `http(s)://` URL, and refuse a cloud storage source while planning.

Packages run to gigabytes and cannot live in the repository. Manifests and images are a few kilobytes and can.

## Credentials

Credentials never go on the package. They come from the machine, or from the provider's `file_sources` block:

```hcl
provider "jamfplatform" {
  file_sources = {
    s3    = { ... }
    azure = { ... }
    https = [ ... ]
  }
}
```

Every part is optional. The `s3` and `azure` blocks take the argument names of Terraform's `s3` and `azurerm` state backends and read the same environment variables, so a machine that already authenticates one of those backends needs nothing more.

Values in the block are not written to state. They are carried in a saved plan file, so prefer a role, a profile or a federated credential to a key written in configuration.

## Amazon S3

The source is `s3://bucket/key`. The key is read as written: nothing is percent-decoded. Add `?versionId=` to read one version of an object on a versioned bucket.

### Permissions

The identity needs to read the object and to list the bucket:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:GetObjectVersion"],
      "Resource": "arn:aws:s3:::my-packages/releases/*"
    },
    {
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::my-packages"
    }
  ]
}
```

- `s3:GetObjectVersion` is needed only for a `?versionId=` source.
- Without `s3:ListBucket`, S3 answers "access denied" for an object that does not exist, and the provider cannot tell a mistyped key from a missing permission.
- An object encrypted with a KMS key also needs `kms:Decrypt` on that key.

### Authentication

With no `s3` block the AWS default credential chain applies: environment variables, the shared configuration files, a container or instance role.

A named profile:

```hcl
file_sources = {
  s3 = {
    profile = "packages"
  }
}
```

A role assumed with the machine's own credentials:

```hcl
file_sources = {
  s3 = {
    assume_role = {
      role_arn    = "arn:aws:iam::123456789012:role/package-read"
      external_id = "terraform"
    }
  }
}
```

A role assumed with an OpenID Connect token, the usual arrangement on a CI runner:

```hcl
file_sources = {
  s3 = {
    assume_role_with_web_identity = {
      role_arn                = "arn:aws:iam::123456789012:role/package-read"
      web_identity_token_file = "/var/run/secrets/token"
    }
  }
}
```

On GitHub Actions, `aws-actions/configure-aws-credentials` exports credentials the default chain picks up, and no `s3` block is needed.

Static keys (`access_key`, `secret_key`, `token`) are supported. Temporary keys set that way are not refreshed, so they must outlive the upload.

### Region

A bucket's Region is found from the first response, so `region` only sets where the client starts. Set it to the bucket's own Region for a bucket whose name holds a dot, which cannot be looked up.

### S3-compatible storage

```hcl
file_sources = {
  s3 = {
    endpoints      = { s3 = "https://minio.internal:9000" }
    use_path_style = true
    access_key     = var.minio_access_key
    secret_key     = var.minio_secret_key
  }
}
```

With a custom endpoint, credentials are not validated against AWS first; a wrong key is reported by the first request to the store. The store must return an ETag and honour `If-Match`.

## Azure Blob Storage

The source is `azblob://account/container/blob`. Give the storage account's name, without `.blob.core.windows.net`. Authentication is through Microsoft Entra ID. A storage account key is not read, and a SAS URL is an `https://` source.

### Permissions

Assign **Storage Blob Data Reader** to the identity, on the container or the storage account. A new role assignment can take a minute to apply.

### Authentication

One method is used, in this order: a client secret, `use_oidc`, `use_msi`, `use_cli`. A method that is switched on but incomplete, or that fails, is an error. It does not fall through to the next.

With no `azure` block and no `ARM_*` variables set, the account signed in to the Azure CLI is used.

A federated credential on an app registration, the usual arrangement on a CI runner:

```hcl
file_sources = {
  azure = {
    tenant_id = "00000000-0000-0000-0000-000000000000"
    client_id = "11111111-1111-1111-1111-111111111111"
    use_oidc  = true
  }
}
```

On GitHub Actions a job with `id-token: write` needs nothing else: the token is requested from the runner. Elsewhere, supply it in `oidc_token` or `oidc_token_file_path`. A token supplied that way is exchanged once more when the access token it bought nears expiry, about an hour on. A CI token has expired by then, so an apply that long needs the request form, a client secret or a managed identity.

A client secret:

```hcl
file_sources = {
  azure = {
    tenant_id     = "00000000-0000-0000-0000-000000000000"
    client_id     = "11111111-1111-1111-1111-111111111111"
    client_secret = var.azure_client_secret
  }
}
```

A managed identity, on compute hosted in Azure:

```hcl
file_sources = {
  azure = {
    use_msi = true
  }
}
```

Each of these can be set with the `azurerm` backend's environment variables instead: `ARM_TENANT_ID`, `ARM_CLIENT_ID`, `ARM_CLIENT_SECRET`, `ARM_USE_OIDC`, `ARM_USE_MSI`, `ARM_USE_CLI` and the rest named on the attributes. The `AZURE_*` variables other tools read are not used.

Three of the backend's methods are not supported: a client certificate, AKS workload identity and an Azure DevOps service connection. A machine set up for one of them is refused by name. Set `use_cli = true` to read with the Azure CLI login there.

`environment` selects a sovereign cloud (`china`, `usgovernment`). Those have not been tested against a real account.

## Federated credentials from GitHub Actions

AWS and Microsoft Entra ID each match a job's OIDC token on its issuer and its subject, character for character. Read both from a real token of the repository before you write the trust policy or the federated credential:

- **Issuer.** `https://token.actions.githubusercontent.com` for most repositories. A repository in a GitHub Enterprise with its own issuer has the enterprise's name appended, and to AWS and to Entra that is a different issuer.
- **Subject.** It can carry the numeric owner and repository IDs (`repo:<owner>@<id>/<repo>@<id>:ref:refs/heads/main`) in place of the `repo:<owner>/<repo>:…` form most documentation shows.

A mismatch is reported as `InvalidIdentityToken` by AWS and `AADSTS700213` by Entra. Neither says which of the two was wrong.

## `https` servers that need a credential

```hcl
provider "jamfplatform" {
  file_sources = {
    https = [
      { host = "artifactory.example.com", bearer_token = var.artifactory_token },
      { host = "files.example.com", username = "ci", password = var.files_password },
      { host = "dl.example.com:8443", header = { name = "X-Api-Key", value = var.dl_key } },
    ]
  }
}
```

A package whose `https://` source is on one of those hosts is downloaded with that host's credential.

- A credential belongs to one hostname and port. It is not sent to a subdomain or to another port.
- It is sent over `https` only. An `http://` source on the same host is downloaded without it.
- On a redirect it is dropped. The host redirected to gets its own credential if it has an entry, and nothing otherwise.
- The host is written in ASCII, without a scheme or a path.

A change to a file behind a URL does not show in the plan, with or without a credential.

## Change detection

For an `s3://` or `azblob://` source the provider records the object's ETag in `package_file_source_version` and compares it on every plan, with one metadata request and no download.

- **Unchanged object.** Nothing is read and nothing is uploaded. An update to `notes` or `priority` changes the record alone.
- **Changed object.** The plan shows `package_file_source_version` changing to the new ETag and the hash attributes as `(known after apply)`. The apply reads the object pinned to that ETag and uploads it. The plan also warns that the file may not need uploading, because a metadata edit or an identical re-upload changes the ETag too. Size and hashes show as `(known after apply)` either way. The apply compares the object with the file Jamf Pro holds before it uploads anything.
- **Object replaced between the plan and the apply.** The apply fails with "Package file changed since the plan" and sends nothing. Run the plan again. When a saved plan is applied after the object has changed, Terraform itself reports the provider's final plan as inconsistent, naming both ETags; the remedy is the same.
- **File replaced in Jamf Pro.** The next refresh clears `package_file_source_version` and warns. The apply reads the object and uploads it if Jamf Pro's file still differs.
- **Object missing while planning.** The plan warns and carries on, because another resource in the same configuration may create the object. The apply fails, before a package record exists, if it is still missing.

An ETag is not a checksum. An Azure ETag changes on every write, including a metadata edit, and an S3 ETag changes when an object is re-uploaded in parts of a different size. A change the provider did not need costs one download: the object is compared with the file Jamf Pro holds, and uploaded only if it differs. With `stream_url_directly`, the provider compares only an object the same size as Jamf Pro's file; see [Streaming](#streaming).

A source with `?versionId=` names one version and is never examined again once it has been read.

### Adopting an existing package

Importing a package, or moving one from a local path to cloud storage, does not upload a file Jamf Pro already has. The first apply reads the object once, compares it with the digests Jamf Pro holds, and records the ETag.

### The plan needs read access

The comparison happens while planning, so the identity that runs `terraform plan` needs the same read access as the one that runs `terraform apply`. Where a pipeline plans on pull requests, that identity can learn whether an object exists, its size and its ETag. Scope it as tightly as the apply identity.

## Jamf Pro permissions

To upload a package file, the API integration needs two permissions besides the package ones. Both sit under **Infrastructure**:

| Permission | Without it |
|---|---|
| Cloud Distribution Point → Read | The provider cannot tell which distribution point the tenant uses. It warns "Package uploaded without reading the cloud distribution point" and uploads the file through Jamf Pro instead of straight to the distribution point |
| Jamf Cloud Distribution Service files → Create | On a Jamf Cloud Distribution Point the upload fails, and the error names this permission |

The full list is under **Required Jamf permissions** on the `jamfplatform_pro_package` resource page.

## Shared runners

The provider reads whatever the runner's identity can read. Anyone who can get configuration applied on a runner can name any object that identity reaches as a package source, and the provider uploads it to Jamf Pro. Any Terraform configuration on a shared runner is in that position, and the provider has no allowlist.

Control it around the provider: limit the runner identity's bucket or container permissions, review the configuration, and check the plan with policy, where `package_file_source` is visible.

## Streaming

By default a cloud storage object is copied to a temporary file, hashed, and uploaded from there. The file is private to the user and removed afterwards.

`stream_url_directly = true` sends the object from the store to Jamf Pro without the temporary file. An unchanged object is still not sent, and a read cut partway resumes where it stopped, pinned to the same ETag. Streaming cannot be combined with `package_file_source_checksum`.

A streamed file has no digest until the provider has sent it. When the ETag changes, the provider decides by size:

- **Same size as Jamf Pro's file.** The provider reads the object once and compares it. It uploads nothing if the bytes match. If they differ, it reads the object again and uploads it, so you pay for two downloads.
- **Different size.** The provider uploads the object in one pass and compares nothing.
- **Adopting a package.** The provider reads the object once and compares it, as in [Adopting an existing package](#adopting-an-existing-package).

The size limit is 8 GiB either way.

## Other stores

For a store the provider does not read, generate a signed URL outside the provider and use it as an `https://` source. S3 objects encrypted with customer-provided keys and requester-pays buckets are not supported as `s3://` sources.

A signed URL is a credential. It is stored in state, and one generated on every run changes the source and plans an update each time.

## Troubleshooting

| Diagnostic | Cause |
|---|---|
| Cannot authenticate to cloud storage | No usable credential. The detail names the method that was tried and what was missing |
| Cloud storage refused access to the package file | The identity lacks read access. On S3 without `s3:ListBucket`, the object may also not exist |
| Package file not found in cloud storage | The bucket, container or object name is wrong |
| Package file changed since the plan | The object was replaced after planning. Run the plan again |
| Server refused the package file download | An `https` host answered 401 or 403 to its credential |
| Cloud storage source not supported here | The attribute is not `package_file_source` |
| Package uploaded without reading the cloud distribution point | The integration lacks Cloud Distribution Point → Read. See [Jamf Pro permissions](#jamf-pro-permissions) |
| Package file not examined during this plan | A `file_sources` value comes from a resource that does not exist yet. The apply decides whether to upload |

With `TF_LOG=DEBUG` the provider logs each object it examines and reads, by name. It logs no credential and no query string.

# Every platform environment the organization holds.
data "jamfplatform_account_platform_environments" "all" {}

# The sandboxes, keyed by name.
output "sandbox_environments" {
  value = {
    for e in data.jamfplatform_account_platform_environments.all.platform_environments :
    e.name => e.id if e.type == "SANDBOX"
  }
}

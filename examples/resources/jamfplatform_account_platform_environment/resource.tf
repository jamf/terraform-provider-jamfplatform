# A sandbox platform environment pinned to one region. Tenants assigned to it
# must be in that region.
resource "jamfplatform_account_platform_environment" "sandbox" {
  name   = "Engineering sandbox"
  type   = "SANDBOX"
  region = "EU"
}

# Leave region out for a global platform environment, which accepts tenants from
# any region.
resource "jamfplatform_account_platform_environment" "production" {
  name = "Production"
  type = "PRODUCTION"
}

# The identifier is what the assign_tenants action takes.
output "sandbox_environment_id" {
  value = jamfplatform_account_platform_environment.sandbox.id
}

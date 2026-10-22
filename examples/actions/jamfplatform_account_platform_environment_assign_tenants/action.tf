# Adds tenants to a platform environment. Assigning a tenant cannot be undone,
# and a platform environment that holds a tenant cannot be deleted, so read the
# list before you invoke it. Invoke the action by name:
#
#   terraform apply -invoke='action.jamfplatform_account_platform_environment_assign_tenants.prod'
#
# Before it sends anything, the action checks that each tenant exists, is
# unassigned and is in a region the platform environment accepts. It names every
# tenant that fails. A tenant already in the platform environment is skipped, so
# running it again changes nothing.
action "jamfplatform_account_platform_environment_assign_tenants" "prod" {
  config {
    environment_id = jamfplatform_account_platform_environment.prod.id
    tenant_ids = [
      for t in data.jamfplatform_account_tenants.all.tenants : t.id
      if t.platform_environment_id == null && t.region == "EU" && t.product == "pro"
    ]
  }
}

resource "jamfplatform_account_platform_environment" "prod" {
  name   = "Production"
  type   = "PRODUCTION"
  region = "EU"
}

data "jamfplatform_account_tenants" "all" {}

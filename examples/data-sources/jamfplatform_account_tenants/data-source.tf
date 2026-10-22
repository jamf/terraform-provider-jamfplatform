# Every tenant the organization holds.
data "jamfplatform_account_tenants" "all" {}

# Tenants not yet in a platform environment, grouped by product. These are the
# ones the assign_tenants action can add to one.
output "unassigned_tenants" {
  value = {
    for t in data.jamfplatform_account_tenants.all.tenants :
    t.id => "${t.product} (${t.region})" if t.platform_environment_id == null
  }
}

# What a platform environment holds. The platform environment itself reports no
# tenants, because membership is recorded on the tenant.
data "jamfplatform_account_platform_environment" "production" {
  name = "Production"
}

output "production_tenants" {
  value = [
    for t in data.jamfplatform_account_tenants.all.tenants :
    t.name if t.platform_environment_id == data.jamfplatform_account_platform_environment.production.id
  ]
}

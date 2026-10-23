# Look a platform environment up by the identifier shown in Jamf Account.
data "jamfplatform_account_platform_environment" "by_id" {
  id = "6f1b4d52-2a37-4c0e-9a55-3d7a8c1e90b4"
}

# Or by name. Names do not have to be unique, and a name that matches more than
# one platform environment fails and lists their identifiers.
data "jamfplatform_account_platform_environment" "by_name" {
  name = "Production"
}

output "production_environment_id" {
  value = data.jamfplatform_account_platform_environment.by_name.id
}

# Every platform environment the organization holds, for generating
# configuration from what already exists. Jamf Account offers nothing to filter
# on.
list "jamfplatform_account_platform_environment" "all" {
  provider = jamfplatform

  config {}
}

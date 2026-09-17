# Every smart mobile device group in the tenant.
data "jamfplatform_pro_smart_mobile_device_groups" "all" {}

# Groups whose name starts with a prefix. The argument is passed through as
# written, and `*` is a wildcard.
data "jamfplatform_pro_smart_mobile_device_groups" "loan_pools" {
  filter = [
    {
      selector = "groupName"
      argument = "Loan*"
    },
  ]
}

# Groups in one site. Only an administrator with full access can filter on the
# site; a site-restricted administrator gets their own site regardless.
data "jamfplatform_pro_smart_mobile_device_groups" "campus" {
  filter = [
    {
      selector = "siteId"
      argument = "7"
    },
  ]
}

# Each result carries the membership Jamf Pro currently counts, which is useful
# for finding groups nothing matches.
output "empty_groups" {
  value = [
    for group in data.jamfplatform_pro_smart_mobile_device_groups.all.smart_mobile_device_groups :
    group.name if group.member_count == 0
  ]
}

data "jamfplatform_pro_static_mobile_device_groups" "all" {}

# Name matching is case-insensitive and * is a wildcard.
data "jamfplatform_pro_static_mobile_device_groups" "loaners" {
  filter = [
    {
      selector = "groupName"
      argument = "Loaner*"
    }
  ]
}

# Groups in one site. A sited administrator has this narrowing applied for them.
data "jamfplatform_pro_static_mobile_device_groups" "in_belfast" {
  filter = [
    {
      selector = "siteId"
      argument = "3"
    }
  ]
}

output "all_static_mobile_device_groups" {
  value = data.jamfplatform_pro_static_mobile_device_groups.all.static_mobile_device_groups
}

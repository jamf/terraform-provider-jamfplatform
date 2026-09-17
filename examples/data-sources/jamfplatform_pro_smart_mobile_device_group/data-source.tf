# Look one group up by the identifier Jamf Pro assigned to it.
data "jamfplatform_pro_smart_mobile_device_group" "by_id" {
  id = "19"
}

# Or by display name, matched exactly.
data "jamfplatform_pro_smart_mobile_device_group" "by_name" {
  name = "Supervised iPads"
}

# The criteria come back in evaluation order, which makes this a convenient way
# to read an existing group before bringing it under management.
output "supervised_ipads_criteria" {
  value = data.jamfplatform_pro_smart_mobile_device_group.by_name.criteria
}

# Scope a Jamf Platform construct to the group without importing it.
output "supervised_ipads_platform_id" {
  value = data.jamfplatform_pro_smart_mobile_device_group.by_name.platform_id
}

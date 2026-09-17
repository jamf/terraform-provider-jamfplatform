data "jamfplatform_pro_static_mobile_device_group" "example_by_id" {
  id = "17"
}

data "jamfplatform_pro_static_mobile_device_group" "example_by_name" {
  name = "Loaner iPads"
}

# The singular lookup is where membership comes from: the plural data source
# reports how many devices a group holds, not which ones.
output "loaner_members" {
  value = data.jamfplatform_pro_static_mobile_device_group.example_by_name.assigned_mobile_device_ids
}

output "static_mobile_device_group_example_by_id" {
  value = data.jamfplatform_pro_static_mobile_device_group.example_by_id
}

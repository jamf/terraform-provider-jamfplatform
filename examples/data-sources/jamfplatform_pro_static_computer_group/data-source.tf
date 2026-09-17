data "jamfplatform_pro_static_computer_group" "example_by_id" {
  id = "41"
}

data "jamfplatform_pro_static_computer_group" "example_by_name" {
  name = "Design Macs"
}

# The computers in the group, which only the singular lookup reports.
output "design_macs_members" {
  value = data.jamfplatform_pro_static_computer_group.example_by_name.assigned_computer_ids
}

output "static_computer_group_example_by_id" {
  value = data.jamfplatform_pro_static_computer_group.example_by_id
}

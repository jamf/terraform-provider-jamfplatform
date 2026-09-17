# Read one smart computer group by identifier or by exact name. Supply exactly
# one of the two.
data "jamfplatform_pro_smart_computer_group" "by_id" {
  id = "41"
}

data "jamfplatform_pro_smart_computer_group" "by_name" {
  name = "Recent MacBooks"
}

# Scope a Jamf Platform construct to a group Jamf Pro already owns.
output "recent_macbooks_platform_id" {
  value = data.jamfplatform_pro_smart_computer_group.by_name.platform_id
}

output "recent_macbooks_criteria" {
  value = data.jamfplatform_pro_smart_computer_group.by_name.criteria
}

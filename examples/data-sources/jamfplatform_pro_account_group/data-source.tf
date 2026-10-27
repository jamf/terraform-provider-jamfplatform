data "jamfplatform_pro_account_group" "by_id" {
  id = "8"
}

data "jamfplatform_pro_account_group" "by_name" {
  display_name = "Help Desk"
}

output "account_group_by_id" {
  value = data.jamfplatform_pro_account_group.by_id
}

output "account_group_by_name" {
  value = data.jamfplatform_pro_account_group.by_name
}

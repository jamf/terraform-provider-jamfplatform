# Removes a user account from the listed Shared iPads.
#
#   terraform apply -invoke='action.jamfplatform_pro_delete_user.remove'

action "jamfplatform_pro_delete_user" "remove" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]

    # Name one account, or set delete_all_users = true to remove them all.
    user_name = "alex"
  }
}

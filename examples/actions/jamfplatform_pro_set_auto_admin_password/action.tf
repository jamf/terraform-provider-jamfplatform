variable "admin_password" {
  type      = string
  sensitive = true
}

# Set the password on one computer. The action looks up the GUID of the account
# on that Mac. Only accounts that Jamf Pro manages through LAPS resolve.
action "jamfplatform_pro_set_auto_admin_password" "admin" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
    username       = "mdmadmin"
    password       = var.admin_password
  }
}

# Set the same password on every member of a computer group. Each computer gets
# its own GUID lookup. A mobile device group is refused.
data "jamfplatform_device_group" "kiosks" {
  id = "7f4cb2da-c7c4-44bd-a76b-147dd24eb613"
}

action "jamfplatform_pro_set_auto_admin_password" "kiosks" {
  config {
    management_ids = data.jamfplatform_device_group.kiosks.members
    username       = "mdmadmin"
    password       = var.admin_password
  }
}

# Any other local administrator account: pass its GUID, for one computer only.
action "jamfplatform_pro_set_auto_admin_password" "by_guid" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
    guid           = "00000000-0000-0000-0000-000000000000"
    password       = var.admin_password
  }
}

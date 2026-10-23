variable "admin_password" {
  type      = string
  sensitive = true
}

action "jamfplatform_pro_set_auto_admin_password" "admin" {
  config {
    serial_number = "C02XXXXXXXXX"
    guid          = "00000000-0000-0000-0000-000000000000"
    password      = var.admin_password
  }
}

# Membership Terraform owns. The group holds exactly these mobile devices, and
# an apply that shortens the list removes the devices it dropped.
resource "jamfplatform_pro_static_mobile_device_group" "loaners" {
  name        = "Loaner iPads"
  description = "Kept at the front desk"

  assigned_mobile_device_ids = ["41", "42", "43"]
}

# Membership Terraform leaves alone. Without the attribute, whoever is in the
# group stays in it and Jamf Pro admins can keep adding and removing devices.
resource "jamfplatform_pro_static_mobile_device_group" "field_kit" {
  name = "Field Kit"
}

# A group that belongs to a site, which is the only thing this resource adds
# over jamfplatform_device_group. The site accepts only mobile devices assigned
# to it, so membership here names devices from that site.
resource "jamfplatform_pro_site" "belfast" {
  name = "Belfast"
}

resource "jamfplatform_pro_static_mobile_device_group" "belfast_spares" {
  name    = "Belfast Spares"
  site_id = jamfplatform_pro_site.belfast.id

  assigned_mobile_device_ids = ["57"]
}

# The identifier a Jamf Platform construct references the group by.
output "loaners_platform_id" {
  value = jamfplatform_pro_static_mobile_device_group.loaners.platform_id
}

# A static computer group whose membership Terraform owns. The set is the whole
# membership: a computer you take out of it leaves the group on the next apply.
resource "jamfplatform_pro_static_computer_group" "design_macs" {
  name        = "Design Macs"
  description = "Colour-managed workstations"

  assigned_computer_ids = ["12", "34", "56"]
}

# The same group with membership left to Jamf Pro. Omitting the attribute is how
# you say that: computers assigned in the admin UI stay put, and an apply that
# changes the name or the description leaves them alone. Set it to [] instead to
# empty the group.
resource "jamfplatform_pro_static_computer_group" "loaner_macs" {
  name = "Loaner Macs"
}

resource "jamfplatform_pro_site" "campus_north" {
  name = "Campus North"
}

# A group that belongs to a Jamf Pro site, which is the one thing
# jamfplatform_device_group cannot do. A sited group accepts only computers
# assigned to that same site.
resource "jamfplatform_pro_static_computer_group" "campus_north_macs" {
  name    = "Campus North Macs"
  site_id = jamfplatform_pro_site.campus_north.id

  assigned_computer_ids = ["78"]
}

# Reference the group from Jamf Platform by its platform identifier.
output "design_macs_platform_id" {
  value = jamfplatform_pro_static_computer_group.design_macs.platform_id
}

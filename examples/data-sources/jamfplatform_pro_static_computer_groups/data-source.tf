data "jamfplatform_pro_static_computer_groups" "all" {}

# Groups whose name starts with "Design".
data "jamfplatform_pro_static_computer_groups" "design" {
  filter = [
    {
      selector = "name"
      argument = "Design*"
    }
  ]
}

# Groups belonging to one Jamf Pro site.
data "jamfplatform_pro_static_computer_groups" "campus_north" {
  filter = [
    {
      selector = "siteId"
      argument = "3"
    }
  ]
}

# Each result reports how many computers the group holds. To read which
# computers those are, look the group up with
# jamfplatform_pro_static_computer_group.
output "design_group_sizes" {
  value = {
    for group in data.jamfplatform_pro_static_computer_groups.design.static_computer_groups :
    group.name => group.member_count
  }
}

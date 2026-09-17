# Smart mobile device group. Jamf Pro decides who is in it by evaluating the
# criteria against mobile device inventory, so no member list appears here.
#
# Criteria are evaluated in the order they are written. Each entry joins the one
# before it with its own and_or value, so the first entry's and_or is never used
# and moving an entry changes who is in the group.
resource "jamfplatform_pro_smart_mobile_device_group" "supervised_ipads" {
  name        = "Supervised iPads"
  description = "Every supervised iPad in the estate"

  criteria = [
    {
      name        = "Model"
      search_type = "like"
      value       = "iPad"
    },
    {
      name        = "Supervised"
      search_type = "is"
      value       = "true"
      and_or      = "and"
    },
  ]
}

# A group that belongs to a site. This is the only reason to reach for this
# resource instead of jamfplatform_device_group.
resource "jamfplatform_pro_site" "campus" {
  name = "North Campus"
}

resource "jamfplatform_pro_smart_mobile_device_group" "campus_loan_pool" {
  name    = "North Campus Loan Pool"
  site_id = jamfplatform_pro_site.campus.id

  criteria = [
    {
      name        = "Asset Tag"
      search_type = "like"
      value       = "LOAN-"
    },
  ]
}

# Parentheses group criteria, which is how an "A and (B or C)" rule is written.
resource "jamfplatform_pro_smart_mobile_device_group" "shared_tablets" {
  name = "Shared Tablets Needing Attention"

  criteria = [
    {
      name        = "Model"
      search_type = "like"
      value       = "iPad"
    },
    {
      name                    = "Last Inventory Update"
      search_type             = "more than x days ago"
      value                   = "30"
      and_or                  = "and"
      has_opening_parenthesis = true
    },
    {
      name                    = "Battery Level"
      search_type             = "less than"
      value                   = "20"
      and_or                  = "or"
      has_closing_parenthesis = true
    },
  ]
}

# Reference the group from a Jamf Platform construct through platform_id.
output "supervised_ipads_platform_id" {
  value = jamfplatform_pro_smart_mobile_device_group.supervised_ipads.platform_id
}

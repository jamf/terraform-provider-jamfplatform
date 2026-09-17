# Every smart computer group in the tenant.
data "jamfplatform_pro_smart_computer_groups" "all" {}

# Groups whose name contains "MacBook". * is a wildcard and the match ignores
# case.
data "jamfplatform_pro_smart_computer_groups" "macbooks" {
  filter = [
    {
      selector = "name"
      argument = "*MacBook*"
    }
  ]
}

# Groups belonging to one site. Filtering on siteId works only for an account
# with full access; a site-restricted account has the filter applied for it.
data "jamfplatform_pro_smart_computer_groups" "eau_claire" {
  filter = [
    {
      selector = "siteId"
      argument = "7"
    }
  ]
}

# Each result reports its membership count and omits its criteria. Read one
# group with jamfplatform_pro_smart_computer_group when you need them.
output "macbook_group_sizes" {
  value = {
    for group in data.jamfplatform_pro_smart_computer_groups.macbooks.smart_computer_groups :
    group.name => group.member_count
  }
}

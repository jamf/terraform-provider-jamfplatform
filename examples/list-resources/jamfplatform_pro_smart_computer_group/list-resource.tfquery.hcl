# Every smart computer group in the tenant.
list "jamfplatform_pro_smart_computer_group" "all" {
  provider = jamfplatform
}

# Groups whose name contains "MacBook", each carrying its full configuration.
# include_resource costs one extra read per group, because a search omits the
# criteria.
list "jamfplatform_pro_smart_computer_group" "macbooks" {
  provider         = jamfplatform
  include_resource = true

  config {
    filter = [
      {
        selector = "name"
        argument = "*MacBook*"
      }
    ]
  }
}

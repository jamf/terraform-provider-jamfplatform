# Every static mobile device group in the tenant.
list "jamfplatform_pro_static_mobile_device_group" "all" {
  provider = jamfplatform
}

# Groups whose name starts with "Loaner", with the resource body generated.
# Membership is left undeclared: the query reads one page and never fetches a
# group's members, so applying the generated configuration leaves them alone.
list "jamfplatform_pro_static_mobile_device_group" "loaners" {
  provider         = jamfplatform
  include_resource = true

  config {
    filter = [
      {
        selector = "groupName"
        argument = "Loaner*"
      }
    ]
  }
}

# Every smart mobile device group in the tenant.
list "jamfplatform_pro_smart_mobile_device_group" "all" {
  provider = jamfplatform
}

# Generate configuration for the groups in one site. include_resource fetches
# each group individually so the exported configuration carries its criteria.
list "jamfplatform_pro_smart_mobile_device_group" "campus" {
  provider         = jamfplatform
  include_resource = true

  config {
    filter = [
      {
        selector = "siteId"
        argument = "7"
      },
    ]
  }
}

# Every Jamf Pro static computer group. Generated configurations leave
# assigned_computer_ids out, so membership stays as Jamf Pro holds it until you
# declare otherwise.
list "jamfplatform_pro_static_computer_group" "all" {
  provider = jamfplatform
}

# Groups whose name starts with "Design".
list "jamfplatform_pro_static_computer_group" "design" {
  provider = jamfplatform

  config {
    filter = [
      {
        selector = "name"
        argument = "Design*"
      }
    ]
  }
}

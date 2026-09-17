# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MPL-2.0

# Import an existing static mobile device group by its Jamf Pro identifier.
#
# Import records the mobile devices Jamf Pro reports in the group even when your
# configuration does not declare `assigned_mobile_device_ids`, so the first plan
# afterwards shows that set going to null. Applying it leaves Jamf Pro's
# membership exactly as it was: the group keeps its devices and Terraform stops
# managing who is in it. Declare `assigned_mobile_device_ids` only if you want
# Terraform to own the membership from then on, naming every device the group
# should hold.
terraform import jamfplatform_pro_static_mobile_device_group.example "17"

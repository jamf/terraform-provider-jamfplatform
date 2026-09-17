# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MPL-2.0

# Import an existing smart mobile device group by the identifier Jamf Pro
# assigned to it.
#
# The identifier the jamfplatform_device_* constructs use is resolved on the
# first refresh after the import, so `platform_id` is populated without any
# further action.
terraform import jamfplatform_pro_smart_mobile_device_group.example "19"

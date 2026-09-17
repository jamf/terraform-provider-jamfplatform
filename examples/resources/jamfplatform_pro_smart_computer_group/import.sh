# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MPL-2.0

# Import an existing smart computer group by its Jamf Pro identifier.
#
# The import resolves platform_id as well, which costs one extra read because no
# group response carries it.
terraform import jamfplatform_pro_smart_computer_group.example "41"

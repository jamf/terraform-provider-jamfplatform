# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MPL-2.0

# Import an existing static computer group by its Jamf Pro identifier.
#
# Import records the computers Jamf Pro reports in the group even when your
# configuration does not declare them, so the first plan afterwards can propose
# removing them. Add assigned_computer_ids to the configuration, or leave the
# attribute out and the membership unmanaged, before applying that plan.
terraform import jamfplatform_pro_static_computer_group.example "41"

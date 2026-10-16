# Fill the app record from its App Store listing. The data source returns the
# name, version, bundle ID, store URL, description and icon for the Mac listing.
data "jamfplatform_apple_app_store_app" "pages" {
  id     = "361309726"
  target = "mac"
}

resource "jamfplatform_pro_icon" "pages" {
  icon_file_source = data.jamfplatform_apple_app_store_app.pages.icon_url
}

# Self Service distribution scoped to every computer, with the App Store
# description and icon. deployment_type defaults to "Make Available in Self
# Service".
resource "jamfplatform_pro_mac_app_store_app" "pages" {
  general = {
    name        = data.jamfplatform_apple_app_store_app.pages.name
    version     = data.jamfplatform_apple_app_store_app.pages.version
    bundle_id   = data.jamfplatform_apple_app_store_app.pages.bundle_id
    url         = data.jamfplatform_apple_app_store_app.pages.url
    is_free     = data.jamfplatform_apple_app_store_app.pages.is_free
    category_id = jamfplatform_pro_category.productivity.id
  }

  scope = {
    targets = {
      all_computers = true
    }
  }

  self_service = {
    install_button_text      = "Install"
    self_service_description = data.jamfplatform_apple_app_store_app.pages.description
    feature_on_main_page     = true

    self_service_icon = {
      id = jamfplatform_pro_icon.pages.id
    }

    self_service_categories = [
      {
        id         = jamfplatform_pro_category.productivity.id
        display_in = true
        feature_in = false
      },
    ]
  }

  # The listing's version changes with each App Store release. Ignore it to
  # keep releases out of plans.
  lifecycle {
    ignore_changes = [general.version]
  }
}

# Automatic install scoped to a computer group, sourced from a Platform
# Services device group via .jamf_pro_id. This lookup uses the bundle ID.
data "jamfplatform_apple_app_store_app" "xcode" {
  bundle_id = "com.apple.dt.Xcode"
  target    = "mac"
}

resource "jamfplatform_pro_mac_app_store_app" "xcode" {
  general = {
    name            = data.jamfplatform_apple_app_store_app.xcode.name
    version         = data.jamfplatform_apple_app_store_app.xcode.version
    bundle_id       = data.jamfplatform_apple_app_store_app.xcode.bundle_id
    url             = data.jamfplatform_apple_app_store_app.xcode.url
    deployment_type = "Install Automatically/Prompt Users to Install"
  }

  scope = {
    targets = {
      computer_group_ids = [
        jamfplatform_device_group.engineering.jamf_pro_id,
      ]
    }

    exclusions = {
      department_ids = [jamfplatform_pro_department.it.id]
    }
  }

  lifecycle {
    ignore_changes = [general.version]
  }
}

# Without the data source, you write each general value by hand. Jamf Pro
# stores it as written and checks nothing against the App Store.
resource "jamfplatform_pro_mac_app_store_app" "manual" {
  general = {
    name      = "Things 3"
    version   = "3.24.1"
    bundle_id = "com.culturedcode.ThingsMac"
    url       = "https://apps.apple.com/us/app/things-3/id904280696?mt=12"
  }
}

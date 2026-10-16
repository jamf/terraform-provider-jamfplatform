# Fill the app record from its App Store listing. The data source returns the
# name, version, bundle ID, store URL, description and icon for the iPhone and
# iPad listing in the chosen country or region.
data "jamfplatform_apple_app_store_app" "pages" {
  id             = "361309726"
  target         = "mobile_device"
  country_region = "US"
}

resource "jamfplatform_pro_icon" "pages" {
  icon_file_source = data.jamfplatform_apple_app_store_app.pages.icon_url
}

# Self Service distribution scoped to every mobile device, with the App Store
# description and icon. deployment_type defaults to "Make Available in Self
# Service".
resource "jamfplatform_pro_mobile_device_app" "pages" {
  general = {
    name                  = data.jamfplatform_apple_app_store_app.pages.name
    version               = data.jamfplatform_apple_app_store_app.pages.version
    bundle_id             = data.jamfplatform_apple_app_store_app.pages.bundle_id
    itunes_store_url      = data.jamfplatform_apple_app_store_app.pages.url
    itunes_country_region = data.jamfplatform_apple_app_store_app.pages.country_region
    is_free               = data.jamfplatform_apple_app_store_app.pages.is_free
    category_id           = jamfplatform_pro_category.productivity.id
  }

  scope = {
    targets = {
      all_mobile_devices = true
    }
  }

  self_service = {
    install_button_text       = "Install"
    after_install_button_text = "Open"
    self_service_description  = data.jamfplatform_apple_app_store_app.pages.description
    feature_on_main_page      = true
    notification_enabled      = true

    self_service_icon = {
      id = jamfplatform_pro_icon.pages.id
    }

    self_service_categories = [
      {
        id         = jamfplatform_pro_category.productivity.id
        display_in = true
      },
    ]
  }

  # The listing's version changes with each App Store release. Ignore it to
  # keep releases out of plans.
  lifecycle {
    ignore_changes = [general.version]
  }
}

# Automatic install scoped to a mobile device group (sourced from a Platform
# Services device group via .jamf_pro_id), with managed app configuration. An
# App Store link works as the lookup key too, and sets the country or region.
data "jamfplatform_apple_app_store_app" "numbers" {
  store_url = "https://apps.apple.com/gb/app/numbers/id361304891"
  target    = "mobile_device"
}

resource "jamfplatform_pro_mobile_device_app" "numbers" {
  general = {
    name                  = data.jamfplatform_apple_app_store_app.numbers.name
    version               = data.jamfplatform_apple_app_store_app.numbers.version
    bundle_id             = data.jamfplatform_apple_app_store_app.numbers.bundle_id
    itunes_store_url      = data.jamfplatform_apple_app_store_app.numbers.url
    itunes_country_region = data.jamfplatform_apple_app_store_app.numbers.country_region
    deployment_type       = "Install Automatically/Prompt Users to Install"
    deploy_as_managed_app = true
  }

  scope = {
    targets = {
      mobile_device_group_ids = [
        jamfplatform_device_group.field_ipads.jamf_pro_id,
      ]
    }

    exclusions = {
      department_ids = [jamfplatform_pro_department.it.id]
    }
  }

  app_configuration = {
    preferences = <<-EOT
      <dict>
        <key>ServerURL</key>
        <string>https://example.com</string>
      </dict>
    EOT
  }

  lifecycle {
    ignore_changes = [general.version]
  }
}

# Without the data source, you write name, version and bundle_id by hand. An
# itunes_store_url marks the record as an App Store app, so it needs no
# os_type. Only in-house apps, which have no store URL, need one.
resource "jamfplatform_pro_mobile_device_app" "manual" {
  general = {
    name             = "Jamf Parent"
    version          = "5.2.8"
    bundle_id        = "com.jamf.parent"
    itunes_store_url = "https://apps.apple.com/us/app/jamf-parent/id1458797105"
  }
}

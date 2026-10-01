# A custom DNS zone resolves hostnames under its domains through name servers you
# nominate, instead of public DNS. Adding one is a prerequisite for reaching
# enterprise apps on internal private networks over ZTNA.

# gateway_id names the gateway each name server is reachable through. Resolve it
# from the Jamf-managed shared gateway catalogue rather than hard-coding the
# four-character ID; a dedicated gateway of your own works the same way, via
# jamfplatform_security_cloud_ztna_gateway.
data "jamfplatform_security_cloud_ztna_shared_gateways" "all" {}

locals {
  nearest_data_center = one([
    for gateway in data.jamfplatform_security_cloud_ztna_shared_gateways.all.shared_gateways :
    gateway.id if gateway.name == "Nearest Data Center"
  ])

  uk_shared_pool = one([
    for gateway in data.jamfplatform_security_cloud_ztna_shared_gateways.all.shared_gateways :
    gateway.id if gateway.name == "Shared IP Pool: Europe - UK"
  ])
}

resource "jamfplatform_security_cloud_dns_zone" "internal" {
  name = "Internal Services"

  # A wildcard covers only subdomains, so list the parent domain alongside it.
  domains = [
    "corp.example.com",
    "*.corp.example.com",
  ]

  # Each IP address may appear only once in a zone.
  authoritative_name_servers = [
    {
      ip_address = "203.0.113.53"
      gateway_id = local.nearest_data_center
    },
    {
      ip_address = "198.51.100.53"
      gateway_id = local.uk_shared_pool
    },
  ]
}

# Reaching a resolver that only exists behind your own IPSec tunnel means pointing
# the name server at that gateway instead.
resource "jamfplatform_security_cloud_dns_zone" "private" {
  name    = "Private Apps"
  domains = ["private.example.com"]

  authoritative_name_servers = [
    {
      ip_address = "10.100.0.53"
      gateway_id = jamfplatform_security_cloud_ztna_gateway.private_apps.id
    },
  ]
}

resource "jamfplatform_security_cloud_ztna_gateway" "private_apps" {
  name          = "Private Apps Egress"
  egress_region = "Europe - UK"

  contact = {
    name  = "Network Operations"
    email = "netops@example.com"
  }
}

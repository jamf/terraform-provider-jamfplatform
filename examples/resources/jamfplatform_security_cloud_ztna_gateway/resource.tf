# A gateway takes one of two forms, chosen by whether the ipsec block is present.

# Form 1: dedicated internet gateway. Omit ipsec, and Jamf Security Cloud
# provisions a pair of private egress IP addresses, reported in
# dedicated_egress_ip_addresses.
resource "jamfplatform_security_cloud_ztna_gateway" "internet" {
  name          = "London Internet Egress"
  egress_region = "Europe - UK"

  # tenant_ids is left unset, so the gateway is granted to your own Security
  # Cloud tenant. Set it to grant other tenants in your organization as well.

  contact = {
    name  = "Network Operations"
    email = "netops@example.com"
  }
}

# Form 2: dedicated IPSec gateway. Set ipsec to build a tunnel to your own VPN
# concentrator. Adding or removing the block later replaces the gateway: Jamf
# Security Cloud will not convert one form into the other.
resource "jamfplatform_security_cloud_ztna_gateway" "ipsec" {
  name          = "Frankfurt Private Apps"
  egress_region = "Europe - Germany"

  contact = {
    name  = "Network Operations"
    email = "netops@example.com"
  }

  # The addresses your firewall must allow. The admin UI lists the ones each
  # egress region offers when you pick the region; supply both for dynamic
  # addressing, or one to pin a single source.
  ipsec_source_ip_addresses = ["3.66.107.208", "3.121.43.105"]

  ipsec = {
    key_exchange_protocol = "IKEv2"

    # Every value below is the label the admin UI shows, not the value stored
    # behind it.
    phase_1 = {
      encryption           = "AES-256"
      integrity            = "SHA-512"
      diffie_hellman_group = "Group 14 (modp2048)"
      sa_lifetime_seconds  = 28800
    }

    phase_2 = {
      encryption           = "AES-256"
      integrity            = "SHA-512"
      diffie_hellman_group = "Group 14 (modp2048)"
      sa_lifetime_seconds  = 28800
    }

    jamf_side = {
      host          = "%any"
      ike_domain_id = "wpa.wandera.com"

      # Exactly one private range, and it must not exist anywhere else on your
      # network: 10.0.0.0/8 (/8-/30), 172.16.0.0/12 (/12-/30) or
      # 192.168.0.0/16 (/16-/30).
      subnet = "172.16.0.0/12"

      # Never stored in Terraform state. Bump authentication_secret_wo_version to
      # rotate it; leaving the version alone keeps the key Jamf Security Cloud
      # already has.
      authentication_secret            = var.ipsec_authentication_secret
      authentication_secret_wo_version = 1
    }

    customer_side = {
      host          = "203.0.113.10"
      ike_domain_id = "vpn.example.com"
      subnets       = ["10.100.0.0/16", "10.101.0.0/16"]

      # Case-sensitive. See the schema for the full list.
      vendor = "strongSwan"
    }
  }
}

variable "ipsec_authentication_secret" {
  description = "IPSec pre-shared key."
  type        = string
  sensitive   = true
}

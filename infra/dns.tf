# Custom domains: boozoo.top (zone on Cloudflare).
#
# Two-step dance per hostname, order matters:
#   1. Cloudflare CNAME  <host> → <container>.functions.fnc.<region>.scw.cloud
#   2. Scaleway custom-domain binding on the container, which validates
#      the CNAME resolves and then issues a Let's Encrypt certificate.
# The depends_on below encodes that order; without it Scaleway's
# validation races the DNS record and the domain lands in error state.
#
# proxied = false (gray cloud) is deliberate, not an oversight:
# Scaleway terminates TLS with its own Let's Encrypt cert, and both the
# initial issuance and every renewal need to see the CNAME directly.
# Proxying through Cloudflare hides the CNAME and breaks issuance. If
# Cloudflare's WAF/CDN layer is ever wanted in front, that's a separate
# migration (Cloudflare origin certs or proxied + rules), not a flag flip.

resource "cloudflare_dns_record" "web" {
  zone_id = var.cloudflare_zone_id
  name    = var.web_hostname
  type    = "CNAME"
  content = scaleway_container.web.domain_name
  proxied = false
  ttl     = 300
  comment = "order-api web app on Scaleway Serverless Containers (Terraform-managed)"
}

resource "cloudflare_dns_record" "api" {
  zone_id = var.cloudflare_zone_id
  name    = var.api_hostname
  type    = "CNAME"
  content = scaleway_container.api.domain_name
  proxied = false
  ttl     = 300
  comment = "order-api Go API on Scaleway Serverless Containers (Terraform-managed)"
}

resource "scaleway_container_domain" "web" {
  container_id = scaleway_container.web.id
  hostname     = var.web_hostname

  depends_on = [cloudflare_dns_record.web]
}

resource "scaleway_container_domain" "api" {
  container_id = scaleway_container.api.id
  hostname     = var.api_hostname

  depends_on = [cloudflare_dns_record.api]
}

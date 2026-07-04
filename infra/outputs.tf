output "web_url" {
  description = "Public URL of the web app"
  value       = "https://${var.web_hostname}"
}

output "api_url" {
  description = "Public URL of the Go API"
  value       = "https://${var.api_hostname}"
}

output "web_scaleway_endpoint" {
  description = "Underlying Scaleway endpoint of the web container (CNAME target)"
  value       = scaleway_container.web.domain_name
}

output "api_scaleway_endpoint" {
  description = "Underlying Scaleway endpoint of the api container (CNAME target)"
  value       = scaleway_container.api.domain_name
}

output "zitadel_project_id" {
  description = "Zitadel project ID (audience scope)"
  value       = zitadel_project.order_api.id
}

output "zitadel_api_client_id" {
  description = "Client ID the Go API validates token audiences against"
  value       = zitadel_application_api.backend.client_id
}

output "zitadel_web_client_id" {
  description = "OIDC client ID of the web app"
  value       = zitadel_application_oidc.web.client_id
}

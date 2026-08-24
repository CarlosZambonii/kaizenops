variable "newrelic_account_id" {
  description = "Account ID numérico da conta New Relic onde o dashboard e as alert policies são criados"
  type        = number
}

variable "newrelic_api_key" {
  description = "User API Key da New Relic (formato NRAK-...), com permissão para criar dashboards e alert policies"
  type        = string
  sensitive   = true
}

variable "newrelic_region" {
  description = "Região da conta New Relic: \"US\" ou \"EU\""
  type        = string
  default     = "US"

  validation {
    condition     = contains(["US", "EU"], var.newrelic_region)
    error_message = "newrelic_region deve ser \"US\" ou \"EU\"."
  }
}

variable "dashboard_name" {
  description = "Nome do dashboard exibido na New Relic"
  type        = string
  default     = "KaizenOps SPC — A3/DMAIC"
}

variable "dashboard_permissions" {
  description = "Visibilidade do dashboard dentro da conta New Relic (public_read_only, private ou public_read_write)"
  type        = string
  default     = "public_read_only"
}

variable "nelson_violation_lookback" {
  description = "Janela usada nos widgets e no NRQL de alerta para considerar uma violação de regra de Nelson como recente"
  type        = string
  default     = "5 minutes"
}

variable "cluster_name" {
  description = "Nome do cluster Kind"
  type        = string
  default     = "kaizenops"
}

variable "namespace" {
  description = "Namespace onde os componentes do KaizenOps rodam"
  type        = string
  default     = "kaizenops"
}

variable "timescaledb_storage_size" {
  description = "Tamanho do volume persistente do TimescaleDB (mantido pequeno por restrição de disco local)"
  type        = string
  default     = "2Gi"
}

variable "timescaledb_superuser_password" {
  description = "Senha do superusuário Postgres/Patroni (dev local apenas — não usar em produção)"
  type        = string
  default     = "kaizenops-dev"
  sensitive   = true
}

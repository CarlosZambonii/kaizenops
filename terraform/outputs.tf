output "cluster_name" {
  value = kind_cluster.kaizenops.name
}

output "kubeconfig_path" {
  value = kind_cluster.kaizenops.kubeconfig_path
}

output "namespace" {
  value = kubernetes_namespace.kaizenops.metadata[0].name
}

output "timescaledb_service" {
  description = "Nome do Service do TimescaleDB dentro do cluster (use com port-forward para acesso local)"
  value       = "${helm_release.timescaledb.name}.${kubernetes_namespace.kaizenops.metadata[0].name}.svc.cluster.local"
}

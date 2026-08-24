provider "kind" {}

resource "kind_cluster" "kaizenops" {
  name           = var.cluster_name
  wait_for_ready = true

  # Cluster de 1 nó só: sem CPU/RAM sobrando para control-plane + workers
  # separados. NetworkPolicies/HPA/RBAC entram na fase 5, não aqui.
  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    node {
      role = "control-plane"
    }
  }
}

provider "kubernetes" {
  host                   = kind_cluster.kaizenops.endpoint
  client_certificate     = kind_cluster.kaizenops.client_certificate
  client_key             = kind_cluster.kaizenops.client_key
  cluster_ca_certificate = kind_cluster.kaizenops.cluster_ca_certificate
}

provider "helm" {
  kubernetes {
    host                   = kind_cluster.kaizenops.endpoint
    client_certificate     = kind_cluster.kaizenops.client_certificate
    client_key             = kind_cluster.kaizenops.client_key
    cluster_ca_certificate = kind_cluster.kaizenops.cluster_ca_certificate
  }
}

resource "kubernetes_namespace" "kaizenops" {
  metadata {
    name = var.namespace
  }

  depends_on = [kind_cluster.kaizenops]
}

resource "helm_release" "timescaledb" {
  name       = "timescaledb"
  namespace  = kubernetes_namespace.kaizenops.metadata[0].name
  repository = "https://charts.timescale.com"
  chart      = "timescaledb-single"
  version    = "0.33.1"

  # Ambiente de laboratório local: 1 réplica, sem backup/exporter, sem TLS
  # customizado. HA de verdade não faz sentido rodando em Kind num note.
  values = [
    yamlencode({
      replicaCount = 1

      resources = {
        requests = {
          cpu    = "250m"
          memory = "512Mi"
        }
        limits = {
          cpu    = "1"
          memory = "1Gi"
        }
      }

      persistentVolumes = {
        data = {
          size = var.timescaledb_storage_size
        }
        wal = {
          size = "1Gi"
        }
      }

      secrets = {
        credentials = {
          PATRONI_SUPERUSER_PASSWORD     = var.timescaledb_superuser_password
          PATRONI_REPLICATION_PASSWORD   = var.timescaledb_superuser_password
          PATRONI_admin_PASSWORD         = var.timescaledb_superuser_password
        }
      }

      backup = {
        enabled = false
      }

      prometheus = {
        enabled = false
      }
    })
  ]

  depends_on = [kubernetes_namespace.kaizenops]
}

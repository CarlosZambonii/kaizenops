# Dashboard KaizenOps no layout A3/DMAIC (Fase 6).
#
# Todas as queries leem o custom event "KaizenOpsSPC", enviado pelo motor SPC
# (Go) já com center_line/ucl/lcl/cpk pré-calculados — o New Relic só plota,
# a inteligência estatística fica no código Go (ADR-3/ADR-4).
#
# Atributos usados: repo, workflow_name, job_name, value, center_line, ucl,
# lcl, cpk, nelson_violation (bool), nelson_rule (número da regra violada).

resource "newrelic_one_dashboard" "kaizenops_spc" {
  name        = var.dashboard_name
  permissions = var.dashboard_permissions

  # -------------------------------------------------------------------------
  # DEFINE — visão geral do processo monitorado: o que está sendo observado
  # e em que volume, sem entrar em estatística ainda.
  # -------------------------------------------------------------------------
  page {
    name = "Define"

    widget_billboard {
      title  = "Builds monitorados (24h)"
      row    = 1
      column = 1
      width  = 4
      height = 3

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT count(*) AS 'Builds' FROM KaizenOpsSPC SINCE 1 day ago"
      }
    }

    widget_billboard {
      title  = "Repositórios com dados de SPC"
      row    = 1
      column = 5
      width  = 4
      height = 3

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT uniqueCount(repo) AS 'Repos' FROM KaizenOpsSPC SINCE 1 week ago"
      }
    }

    widget_table {
      title  = "Repositórios e workflows monitorados"
      row    = 1
      column = 9
      width  = 4
      height = 3

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT count(*) FROM KaizenOpsSPC FACET repo, workflow_name SINCE 1 week ago LIMIT 50"
      }
    }
  }

  # -------------------------------------------------------------------------
  # MEASURE — cartas de controle: value observado contra centerline/UCL/LCL.
  # -------------------------------------------------------------------------
  page {
    name = "Measure"

    widget_line {
      title  = "Carta de controle I-MR — duração de build vs. limites"
      row    = 1
      column = 1
      width  = 8
      height = 4

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT average(value), average(center_line), average(ucl), average(lcl) FROM KaizenOpsSPC TIMESERIES FACET workflow_name SINCE 1 week ago LIMIT 10"
      }
    }

    widget_billboard {
      title  = "Cpk atual por workflow"
      row    = 1
      column = 9
      width  = 4
      height = 4

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT latest(cpk) FROM KaizenOpsSPC FACET workflow_name SINCE 1 hour ago LIMIT 10"
      }
    }
  }

  # -------------------------------------------------------------------------
  # ANALYZE — Pareto de causas de falha (por job, nunca por pessoa) e
  # evolução da capabilidade de processo.
  # -------------------------------------------------------------------------
  page {
    name = "Analyze"

    widget_bar {
      title  = "Pareto de violações de Nelson por job"
      row    = 1
      column = 1
      width  = 6
      height = 4

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT count(*) FROM KaizenOpsSPC WHERE nelson_violation IS TRUE FACET job_name SINCE 1 week ago LIMIT 20"
      }
    }

    widget_line {
      title  = "Cpk ao longo do tempo"
      row    = 1
      column = 7
      width  = 6
      height = 4

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT average(cpk) FROM KaizenOpsSPC TIMESERIES FACET workflow_name SINCE 1 week ago LIMIT 10"
      }
    }
  }

  # -------------------------------------------------------------------------
  # CONTROL — violações de regra de Nelson: quando e com que frequência o
  # processo saiu de controle estatístico.
  # -------------------------------------------------------------------------
  page {
    name = "Control"

    widget_line {
      title  = "Violações de regra de Nelson ao longo do tempo"
      row    = 1
      column = 1
      width  = 8
      height = 4

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT count(*) FROM KaizenOpsSPC WHERE nelson_violation IS TRUE TIMESERIES FACET nelson_rule SINCE 1 week ago"
      }
    }

    widget_table {
      title  = "Violações recentes (detalhe)"
      row    = 1
      column = 9
      width  = 4
      height = 4

      nrql_query {
        account_id = var.newrelic_account_id
        query      = "SELECT repo, workflow_name, job_name, nelson_rule, value, center_line, ucl, lcl FROM KaizenOpsSPC WHERE nelson_violation IS TRUE SINCE 1 day ago LIMIT 100"
      }
    }
  }
}

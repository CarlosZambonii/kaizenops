# Alertas de violação de regra de Nelson (Fase 6).
#
# Política única para todas as violações de causa especial detectadas pelo
# motor SPC. Condição principal dispara para qualquer regra (1 a 4); uma
# segunda condição, faceteada por nelson_rule, mantém o sinal separado por
# regra para quem precisar diferenciar (ex.: regra 1 — ponto fora dos
# limites — normalmente é mais urgente que regras de tendência).
#
# Nenhum destino/canal de notificação é criado aqui: isso depende de conta
# real (Slack, e-mail, PagerDuty, webhook) que este ambiente não tem. Ver
# README.md.

resource "newrelic_alert_policy" "spc_violations" {
  name                = "KaizenOps SPC Violations"
  incident_preference = "PER_CONDITION_AND_TARGET"
}

# Dispara assim que qualquer evento KaizenOpsSPC chega com nelson_violation =
# true. Janela de avaliação curta (5 min) porque o objetivo é agir enquanto o
# processo ainda está fora de controle, não no dia seguinte.
resource "newrelic_nrql_alert_condition" "nelson_violation_any" {
  account_id = var.newrelic_account_id
  policy_id  = newrelic_alert_policy.spc_violations.id

  type                         = "static"
  name                         = "Nelson rule violation detected"
  description                  = "Qualquer regra de Nelson disparou no motor SPC — processo saiu de controle estatístico."
  enabled                      = true
  violation_time_limit_seconds = 3600

  nrql {
    query = "SELECT count(*) FROM KaizenOpsSPC WHERE nelson_violation IS TRUE"
  }

  critical {
    operator              = "above"
    threshold             = 0
    threshold_duration    = 300
    threshold_occurrences = "at_least_once"
  }

  aggregation_window           = 60
  aggregation_method           = "event_flow"
  aggregation_delay            = 120
  fill_option                  = "none"
  expiration_duration          = 3600
  open_violation_on_expiration = false
}

# Mesmo gatilho, mas faceteado por regra — cada nelson_rule vira uma série
# (signal) independente, então o histórico de incidentes já vem separado por
# regra sem precisar de N conditions manuais.
resource "newrelic_nrql_alert_condition" "nelson_violation_by_rule" {
  account_id = var.newrelic_account_id
  policy_id  = newrelic_alert_policy.spc_violations.id

  type                         = "static"
  name                         = "Nelson rule violation detected (by rule)"
  description                  = "Violação de regra de Nelson, com o número da regra como dimensão do sinal — permite ver no incidente qual regra específica disparou."
  enabled                      = true
  violation_time_limit_seconds = 3600

  nrql {
    query = "SELECT count(*) FROM KaizenOpsSPC WHERE nelson_violation IS TRUE FACET nelson_rule"
  }

  critical {
    operator              = "above"
    threshold             = 0
    threshold_duration    = 300
    threshold_occurrences = "at_least_once"
  }

  aggregation_window  = 60
  aggregation_method  = "event_flow"
  aggregation_delay   = 120
  fill_option         = "none"
  expiration_duration = 3600
}

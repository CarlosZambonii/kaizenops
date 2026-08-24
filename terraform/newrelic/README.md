# terraform/newrelic

Terraform do dashboard New Relic (layout A3/DMAIC) e dos alertas de violação
de regra de Nelson do KaizenOps — Fase 6.

> **Nunca foi aplicado de verdade.** Este código foi escrito e formatado sem
> acesso a uma conta New Relic real (sem `NEW_RELIC_API_KEY` nem account ID
> válidos neste ambiente). `terraform init`/`plan`/`apply` não foram
> executados contra uma conta de verdade — apenas a sintaxe HCL foi validada.
> Revise antes do primeiro `apply` real.

## O que este módulo cria

- `newrelic_one_dashboard` — dashboard "KaizenOps SPC — A3/DMAIC" com 4
  páginas:
  - **Define**: volume de builds monitorados e quais repos/workflows têm
    dados de SPC chegando.
  - **Measure**: carta de controle (value/center_line/ucl/lcl) e Cpk atual
    por workflow.
  - **Analyze**: Pareto de violações de Nelson por job e Cpk ao longo do
    tempo.
  - **Control**: contagem de violações de Nelson ao longo do tempo, por
    regra, e uma tabela de detalhe das violações recentes.
- `newrelic_alert_policy` "KaizenOps SPC Violations" com duas
  `newrelic_nrql_alert_condition`:
  - uma condição geral, que dispara para qualquer `nelson_violation = true`;
  - uma condição faceteada por `nelson_rule`, para separar o sinal por regra
    no histórico de incidentes.

  Ambas usam janela de avaliação de 5 minutos, `operator = "above"`,
  `threshold = 0`, `threshold_occurrences = "at_least_once"` — ou seja,
  dispara assim que aparece um único evento de violação na janela.

Todas as queries leem o custom event `KaizenOpsSPC`, enviado pelo motor SPC
(Go) com `repo`, `workflow_name`, `job_name`, `value`, `center_line`, `ucl`,
`lcl`, `cpk`, `nelson_violation` e `nelson_rule` já pré-calculados — o New
Relic apenas plota (ADR-3/ADR-4 do `CLAUDE.md`).

## Variáveis a preencher

| Variável | Obrigatória | Descrição |
|---|---|---|
| `newrelic_account_id` | sim | Account ID numérico da conta New Relic. |
| `newrelic_api_key` | sim (sensitive) | User API Key (`NRAK-...`) com permissão para criar dashboards e alert policies. |
| `newrelic_region` | não (default `"US"`) | `"US"` ou `"EU"`. |
| `dashboard_name` | não | Nome exibido do dashboard. |
| `dashboard_permissions` | não | Visibilidade do dashboard (`public_read_only`, `private`, `public_read_write`). |
| `nelson_violation_lookback` | não | Documental por enquanto — não é referenciada em NRQL hardcoded neste primeiro corte; ajuste as strings `SINCE`/janelas nos arquivos `.tf` se quiser parametrizar de fato. |

`newrelic_account_id`, `newrelic_api_key` e `newrelic_region` também podem
vir das variáveis de ambiente `NEW_RELIC_ACCOUNT_ID`, `NEW_RELIC_API_KEY` e
`NEW_RELIC_REGION` — o provider as lê automaticamente. **Nunca** commitar um
`terraform.tfvars` com a API key preenchida.

## Como usar (quando houver conta real)

```bash
cd terraform/newrelic

export NEW_RELIC_ACCOUNT_ID="123456"
export NEW_RELIC_API_KEY="NRAK-xxxxxxxxxxxxxxxxxxxxxxxxxxx"
export NEW_RELIC_REGION="US"

terraform init
terraform plan
terraform apply
```

Este diretório é independente do `terraform/` raiz (Kind + TimescaleDB): tem
seu próprio state, sua própria versão de provider, e nada aqui referencia o
cluster Kind. Rode os dois `terraform apply` separadamente.

## O que NÃO foi validado

Sem `NEW_RELIC_API_KEY` nem account ID reais neste ambiente:

- `terraform init` não foi executado com o provider real baixado do
  registry (ver nota de rede abaixo) — os nomes de blocos/atributos do
  provider `newrelic/newrelic` (`newrelic_one_dashboard`,
  `newrelic_alert_policy`, `newrelic_nrql_alert_condition` e seus
  sub-blocos: `page`, `widget_billboard`, `widget_line`, `widget_bar`,
  `widget_table`, `nrql_query`, `critical`) foram escritos de memória contra
  a documentação do provider, não confirmados por `terraform validate`.
- `terraform plan`/`apply` não foram rodados — nenhuma chamada real à API
  da New Relic aconteceu.
- As queries NRQL não foram executadas contra dados reais de
  `KaizenOpsSPC` — a existência e os nomes exatos dos atributos
  (`repo`, `workflow_name`, `job_name`, `value`, `center_line`, `ucl`,
  `lcl`, `cpk`, `nelson_violation`, `nelson_rule`) dependem do client Go
  que outro agente está construindo em paralelo; confira os nomes reais
  antes do primeiro `apply`.
- Nenhum canal/destino de notificação (`newrelic_notification_destination`/
  `newrelic_notification_channel` + `newrelic_notification_event`) foi
  criado — a alert policy dispara incidentes, mas sem canal configurado
  eles não notificam ninguém. Isso depende de decidir o destino (Slack,
  e-mail, webhook, PagerDuty), que este ambiente não tem como testar.
- `terraform fmt` foi rodado; `terraform validate` só foi possível na
  medida em que o binário `terraform` e o download do provider estavam
  disponíveis neste ambiente (ver saída do agente que gerou este código).

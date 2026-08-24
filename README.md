# KaizenOps

Plataforma de melhoria contínua de pipelines CI/CD que aplica Statistical
Process Control (Lean Six Sigma) e Machine Learning sobre dados reais de
execução do GitHub Actions.

Licença: AGPL-3.0 · Go (core) + Python (serviço ML)

Ver [`CLAUDE.md`](CLAUDE.md) para a visão completa do projeto (motivação,
arquitetura, ADRs, convenções). Este README é o quickstart e o retrato do
que está implementado hoje.

## Arquitetura

```
GitHub Actions (fonte de dados)
        |
        | webhooks workflow_run / workflow_job
        v
Collector (Go) ----> TimescaleDB <---- SPC Engine (Go)
        |                ^                |
        |                |                | custom events
        |                |                v
        |            ML Service ------------> New Relic
        |          (Python/FastAPI)          (APM + dashboard A3)
        |                ^
        +----------------+
        chamadas /anomaly e /predict (circuit breaker)
```

## Status por fase

| Fase | O que é | Status |
|---|---|---|
| 1 — Infra base | Kind + TimescaleDB via Terraform, Docker Compose para dev local, tfsec/Checkov no CI | ✅ aplicado e testado |
| 2 — Ingestão | GitHub App auth, webhooks HMAC, worker pool, pseudonimização, storage | ✅ testado ponta a ponta com webhooks assinados reais |
| 3 — SPC/DORA | Carta I-MR, regras de Nelson, Cp/Cpk, Pareto, DORA, `/spc/report`, envio ao New Relic | ✅ testado ponta a ponta com dados reais no TimescaleDB |
| 4 — ML | Feature engineering, treino (Isolation Forest), API `/anomaly` `/predict`, client Go com circuit breaker | ✅ testado ponta a ponta (Go ↔ Python reais) |
| 5 — Infra/Segurança | Helm chart, NetworkPolicy, RBAC, PodSecurityStandards, gosec/Trivy/Cosign no CI | ⚠️ ver limitações abaixo |
| 6 — Dashboard/Relatório | `a3report` (Markdown), dashboard New Relic (Terraform) | ✅ relatório testado · ⚠️ dashboard não aplicado |

## O que foi validado de verdade (não só "compila")

- **Collector → TimescaleDB**: webhook `workflow_run`/`workflow_job` assinado
  de verdade (HMAC), autor pseudonimizado, linha chega no banco com
  `duration_seconds`/`queue_seconds` corretos.
- **SPC**: `cmd/spc` rodando contra TimescaleDB real, `/spc/report` retornando
  carta I-MR, violações de regra de Nelson, Cp/Cpk e Pareto calculados sobre
  dados reais da tabela `workflow_jobs`.
- **DORA**: `/spc/report` calculando deployment frequency e change failure
  rate reais a partir de `workflow_runs`.
- **A3 report**: `cmd/a3report` gerando Markdown real a partir do mesmo banco.
- **ML**: `ml/training` treina um Isolation Forest de verdade (com gate de
  qualidade); `ml/api` serve `/anomaly` com o modelo carregado e detecta
  outliers corretamente; `internal/mlclient` (Go) consome esse serviço real,
  incluindo o caminho de degradação graciosa quando o serviço está fora do ar.
- **Infra**: `helm lint` + `helm template` + `kubectl apply --dry-run=server`
  contra a API real do cluster Kind; deploy real do chart mostrando
  `securityContext`, resource limits e probes aplicados corretamente nos
  pods (as imagens não existem em nenhum registry ainda, então os pods ficam
  em `ImagePullBackOff` — esperado).
- **Terraform**: `terraform validate` do módulo `newrelic` passa contra o
  provider real (sem aplicar contra uma conta real). PodSecurityStandards do
  namespace aplicado via `terraform apply` de verdade, sem quebrar o
  TimescaleDB que já estava rodando.

## Limitações conhecidas (não finja que não existem)

- **Nenhum GitHub App real foi criado.** Tudo em `internal/github` foi
  testado com webhooks simulados (assinados manualmente com o secret
  configurado), não com um App instalado num repositório de verdade.
- **Nenhuma conta New Relic real existe neste ambiente.** O client
  (`internal/newrelic`) e o dashboard (`terraform/newrelic/`) foram
  validados sintaticamente e com testes unitários, mas nunca enviaram um
  evento pra uma conta real nem tiveram o dashboard efetivamente aplicado.
- **As imagens Docker nunca foram publicadas em nenhum registry.** Os três
  Dockerfiles (`cmd/collector`, `cmd/spc`, `ml/api`) buildam de verdade e os
  containers rodam (collector serve `/healthz`, ml serve `/healthz` e
  detecta anomalia real, spc falha com erro claro — não crash — sem banco).
  Nenhuma foi carregada num cluster Kind real (a ferramenta
  `kind load docker-image` não funciona neste ambiente sandboxed —
  limitação do ambiente, não do
  Dockerfile).
- **gosec, Trivy e Cosign não rodaram localmente** (não instalados neste
  ambiente) — os workflows de CI (`ci.yml`) foram revisados manualmente e
  validados como YAML sintaticamente correto, mas nunca executados de
  verdade num runner do GitHub Actions.
- **PodSecurityStandards é `baseline`, não `restricted`, no namespace
  compartilhado.** O TimescaleDB (chart de terceiros) não é compatível com
  `restricted` (roda sem `seccompProfile` nem capabilities dropadas); os
  componentes do próprio KaizenOps já rodam com securityContext hardened,
  mas isso não é reforçado por admission control enquanto dividirem
  namespace com o TimescaleDB. Ver comentário em `terraform/main.tf`.
- **DORA lead time e MTTR são sempre 0.** Não há, hoje, correlação
  commit→deploy nem uma fonte de incidents — só deployment frequency e
  change failure rate refletem dado real.
- **Achados de segurança (`internal/security`) usam fixtures de teste
  plausíveis, não relatórios reais** de um `gosec`/`trivy` rodado neste
  repo.

## Quickstart local

```bash
# 1. Sobe TimescaleDB local
docker compose up -d

# 2. Roda o collector (recebe webhooks do GitHub)
export GITHUB_APP_ID=... GITHUB_APP_PRIVATE_KEY="$(cat app.pem)" \
       GITHUB_WEBHOOK_SECRET=... PSEUDONYM_SALT=... \
       DATABASE_URL="postgres://kaizenops:kaizenops-dev@localhost:5432/kaizenops?sslmode=disable"
go run ./cmd/collector

# 3. Roda o motor SPC (expõe /spc/report)
export DATABASE_URL="postgres://kaizenops:kaizenops-dev@localhost:5432/kaizenops?sslmode=disable"
export SPC_UPPER_SPEC_SECONDS=300 DORA_DEPLOY_WORKFLOW=CD
go run ./cmd/spc
curl "http://localhost:8081/spc/report?repo=owner/repo&job=test"

# 4. Gera o relatório A3 do mês
go run ./cmd/a3report --repo owner/repo --job test --workflow CD --out relatorio.md

# 5. Sobe o serviço de ML
python3 -m venv .venv && . .venv/bin/activate
pip install -r ml/requirements.txt
python -m ml.training          # treina um modelo de exemplo
uvicorn ml.api.main:app --reload

# 6. Infra completa (Kind + TimescaleDB + chart do KaizenOps)
cd terraform && terraform apply
helm install kaizenops ../deploy/kaizenops --namespace kaizenops
```

## Estrutura

Ver seção 5 do [`CLAUDE.md`](CLAUDE.md).

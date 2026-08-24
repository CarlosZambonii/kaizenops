# KaizenOps

Plataforma de melhoria contínua de pipelines CI/CD que aplica Statistical
Process Control (Lean Six Sigma) e Machine Learning sobre dados reais de
execução do GitHub Actions.

Licença: AGPL-3.0
Linguagens: Go (core), Python (serviço ML)

---

## 1. Por que este projeto existe

Times de engenharia tratam o pipeline de CI/CD como infraestrutura, não como
processo produtivo. O resultado é que problemas de fluxo (builds que degradam,
testes flaky, filas crescentes, retrabalho por re-run) só são percebidos quando
já viraram bloqueio.

Manufatura resolveu esse problema há décadas com Statistical Process Control:
você mede o processo, separa variação de causa comum de variação de causa
especial, e age apenas quando há sinal estatístico real. O KaizenOps traz esse
ferramental para o fluxo de entrega de software.

Princípio central: melhorar o processo, nunca vigiar pessoas. Toda análise
aponta para testes, jobs e etapas do pipeline. Identidade de contribuidor é
pseudonimizada na ingestão e nunca aparece em dashboard.

Segundo princípio: o ML é enriquecimento, nunca ponto único de falha. Se o
serviço de ML cair, o KaizenOps continua entregando valor apenas com SPC.

---

## 2. O que o sistema faz

1. Coleta métricas de execução de pipelines do GitHub Actions (via GitHub App,
   read-only) e persiste como série temporal.
2. Aplica SPC sobre essas métricas: cartas de controle, regras de Nelson,
   capabilidade de processo (Cp/Cpk), Pareto de causas de falha.
3. Calcula DORA metrics: deployment frequency, lead time for changes, change
   failure rate, MTTR.
4. Detecta anomalias de duração de build com ML não supervisionado e (quando há
   volume de dados suficiente) prevê risco de falha de um build.
5. Publica tudo como custom events e APM no New Relic, com dashboard no layout
   A3/DMAIC e alertas nas violações de regra de Nelson.
6. Comenta automaticamente em Pull Requests quando detecta risco ou anomalia.
7. Gera relatório mensal de melhoria em formato A3 (Markdown).

---

## 3. Arquitetura

```
GitHub Actions (fonte de dados)
        |
        | webhooks workflow_run / workflow_job + REST API
        v
Collector (Go) ----> TimescaleDB <---- SPC Engine (Go)
        |                ^                |
        |                |                | custom events
        |                |                v
        |            ML Service ------------> New Relic
        |          (Python/FastAPI)          (APM + dashboard A3)
        |                ^
        +----------------+
        chamadas /anomaly e /predict
```

Tudo roda em cluster Kind local, provisionado por Terraform.

TimescaleDB é a fonte da verdade. New Relic é camada de visualização e
alerting. Se a conta New Relic sumir, nenhum dado histórico se perde.

---

## 4. Componentes

### 4.1 Collector (Go)

Responsabilidade: ingerir eventos de pipeline e persistir.

- GitHub App com escopo mínimo read-only: `actions:read`, `metadata:read`,
  webhooks `workflow_run` e `workflow_job`. Não lê código, secrets ou settings.
- Autenticação: JWT assinado com chave privada do App, trocado por installation
  token, com renovação automática antes do vencimento.
- Servidor HTTP recebendo webhooks com validação de assinatura HMAC SHA-256.
- Client REST da API do GitHub Actions como fonte complementar (backfill e
  reconciliação), com rate limiting e backoff exponencial.
- Worker pool: goroutines consumindo de channel buffered, com graceful shutdown
  via context cancellation.
- Pseudonimização de autor no ponto de ingestão: hash com salt. O username cru
  nunca chega ao banco.

Dados extraídos por execução: repo, workflow, job, step, status, conclusão,
duração, tempo de fila, branch, evento disparador, número de arquivos alterados,
tipos de arquivo, timestamp, autor pseudonimizado.

### 4.2 SPC Engine (Go)

Responsabilidade: transformar métricas brutas em sinal estatístico.

- Carta Individuals / Moving Range (I-MR) como padrão. Builds chegam um a um
  (n=1 por observação), não em subgrupos racionais, portanto I-MR é a carta
  estatisticamente correta. X-bar/R fica disponível para métricas agregadas por
  dia.
- Regras de Nelson 1 a 4 para detecção de causa especial.
- Cp e Cpk contra spec configurável por repositório (exemplo: build abaixo de
  5 minutos).
- Pareto de causas de falha, agrupado por job e por teste, nunca por pessoa.
- DORA metrics calculadas sobre a mesma base.
- Envia custom events ao New Relic já com UCL, LCL, linha central e Cpk como
  atributos pré-calculados, porque NRQL não computa limites de controle
  nativamente. A inteligência estatística fica no código Go, a ferramenta de
  observabilidade apenas plota.

### 4.3 ML Service (Python / FastAPI / scikit-learn)

Responsabilidade: detectar o que a estatística clássica não captura.

- Feature engineering versionado no repo, usado tanto no treino quanto na
  inferência, para evitar training-serving skew.
- Isolation Forest sobre duração de jobs (não supervisionado, funciona com pouco
  dado). Este é o modelo prioritário.
- Classificador de risco de falha (gradient boosting) apenas quando o volume de
  dados sustentar. Se não sustentar, o projeto entrega valor sem ele.
- Endpoints `/anomaly` e `/predict`.
- Artefato de modelo acompanhado de metadata JSON: data do treino, hash do
  dataset, métricas obtidas, versão do script de features.

### 4.4 Ciclo MLOps

1. Feature engineering lê o TimescaleDB.
2. Treino roda como job no GitHub Actions, não em notebook.
3. Gate de qualidade: o modelo só é publicado se passar no threshold de
   avaliação. Modelo ruim não passa do pipeline, igual código quebrado não passa
   do build.
4. Artefato versionado com metadata (responde "esse modelo foi treinado com quê,
   quando, e com que resultado").
5. Serving via FastAPI. O Go chama com circuit breaker e graceful degradation.
6. Monitoramento de drift usando o próprio SPC: taxa de anomalias detectadas e
   score médio do modelo entram numa carta de controle. Sinal de fora de
   controle significa que o mundo mudou ou o modelo degradou.
7. Retreino disparado por volume de dados novos ou por sinal de drift. Volta ao
   passo 2. Loop fechado, sem intervenção manual.

### 4.5 Infraestrutura

- Cluster Kind provisionado por Terraform.
- TimescaleDB via Helm, com hypertables e migrations versionadas.
- Manifests ou Helm chart próprio: Deployments, Services, Ingress, HPA.
- NetworkPolicies (ML service acessível apenas pelo Collector), RBAC mínimo,
  PodSecurityStandards.
- Docker Compose para desenvolvimento local rápido, antes de subir no Kind.

### 4.6 Segurança no pipeline

- gosec e Trivy no CI.
- tfsec e Checkov validando o Terraform.
- Cosign com assinatura keyless via OIDC do GitHub Actions.
- Twist Lean: findings de segurança viram métricas de qualidade. MTTR de
  vulnerabilidade e taxa de defeito de segurança por release entram nas cartas
  de controle. Segurança medida como processo, não como checklist.

---

## 5. Estrutura de diretórios

```
kaizenops/
  cmd/
    collector/       entrypoint do coletor
    spc/              entrypoint do motor SPC (ou subcomando)
    a3report/         gerador de relatório A3
  internal/
    github/           GitHub App auth, webhooks, client REST
    ingest/           worker pool, pseudonimização, validação
    storage/          repositório TimescaleDB, migrations
    spc/              cartas de controle, regras de Nelson, Cp/Cpk, Pareto
    dora/             cálculo das DORA metrics
    newrelic/         envio de custom events, APM
    mlclient/         client HTTP do ML service, circuit breaker
    config/           carregamento de configuração e secrets
  pkg/                apenas o que fizer sentido expor publicamente
  ml/
    api/              FastAPI
    features/         feature engineering (compartilhado treino/inferência)
    training/         scripts de treino e avaliação
    models/           artefatos versionados + metadata JSON
  terraform/          Kind, namespaces, Helm releases
  deploy/             manifests ou Helm chart do KaizenOps
  migrations/         SQL versionado
  .github/workflows/  CI, treino, retreino
```

---

## 6. Convenções de código

Go:
- Go 1.22 ou superior.
- Erros sempre embrulhados com contexto: `fmt.Errorf("ingesting run: %w", err)`.
- Nada de panic fora de inicialização.
- Context propagado em toda chamada com IO.
- Testes table-driven, padrão da linguagem.
- Interfaces definidas no pacote consumidor, não no produtor.
- Sem framework web pesado, `net/http` e roteador enxuto bastam.
- Configuração via variáveis de ambiente, com validação no startup.

Python:
- Python 3.12, type hints obrigatórios.
- FastAPI com modelos Pydantic para request e response.
- Nenhuma lógica de negócio no arquivo da API, apenas orquestração.

Geral:
- Nenhum secret em código ou em arquivo versionado.
- Toda decisão arquitetural relevante registrada como ADR em `docs/adr/`.
- Commits em inglês, no padrão conventional commits.

---

## 7. Decisões já tomadas (não reabrir sem motivo forte)

- ADR-1: carta I-MR como padrão, não X-bar/R. Motivo: n=1 por observação.
- ADR-2: SPC em Go, ML em Python. SPC é determinístico e simples, implementar em
  Go evita hop de rede e aprofunda a linguagem. ML fica em Python pelo
  ecossistema scikit-learn.
- ADR-3: TimescaleDB como fonte da verdade, New Relic como visualização.
- ADR-4: New Relic em vez de Grafana self-hosted. Menos componente no cluster,
  reaproveita instrumentação APM já validada. Grafana documentado como plano B.
- ADR-5: tudo roda no Kind, sem exceção. Latência não é crítica (webhooks
  toleram segundos), então maximiza valor de laboratório Kubernetes.
- ADR-6: GitHub App em vez de Personal Access Token. Permissão mínima,
  instalável por repo, revogável em um clique.
- ADR-7: pseudonimização na ingestão, não no dashboard. Dado pessoal nunca chega
  cru ao armazenamento.

---

## 8. Ordem de construção

Fase 1: repo, estrutura Go, Terraform (Kind + TimescaleDB via Helm), tfsec e
Checkov no CI, Docker Compose para dev local.
Critério de pronto: `terraform apply` sobe o ambiente completo do zero.

Fase 2: GitHub App, autenticação, webhooks com HMAC, client REST com rate
limiting, worker pool, pseudonimização, schema e migrations, testes.
Critério de pronto: métricas de repositórios reais fluindo para o banco.

Fase 3: motor SPC completo, DORA metrics, envio de custom events ao New Relic.
Critério de pronto: endpoint `/spc/report` funcional e limites de controle
chegando no New Relic.

Fase 4: ML service, Isolation Forest primeiro, classificador só se houver
volume, integração Go com circuit breaker, pipeline de retreino.
Critério de pronto: comentário automático em PR sinalizando risco ou anomalia.

Fase 5: Helm chart, NetworkPolicies, RBAC, PodSecurityStandards, gosec, Trivy,
Cosign, findings virando métricas.
Critério de pronto: KaizenOps rodando no Kind e monitorando o próprio pipeline
de deploy.

Fase 6: dashboard New Relic no layout DMAIC, alertas de regra de Nelson, gerador
de relatório A3 mensal, README, demo.
Critério de pronto: projeto apresentável em menos de 5 minutos.

---

## 9. Como me ajudar neste repo

- Implemente uma fase por vez, na ordem acima. Não pule para frente.
- Antes de escrever código novo, verifique o que já existe no repo.
- Prefira a solução mais simples que atenda ao critério de pronto da fase.
- Ao criar um componente novo, escreva o teste junto, não depois.
- Se uma decisão contrariar um ADR da seção 7, avise antes de implementar.
- Explique escolhas técnicas de forma curta e direta, sem texto de enchimento.

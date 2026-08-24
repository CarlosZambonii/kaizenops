# ADR-6: GitHub App em vez de Personal Access Token

## Status
Aceito

## Contexto
O Collector precisa ler eventos de execução de Actions de repositórios do
GitHub. Um Personal Access Token (PAT) daria acesso amplo, atrelado a uma
conta pessoal, difícil de auditar e revogar seletivamente.

## Decisão
Usar GitHub App com permissões mínimas read-only (`actions:read`,
`metadata:read`), autenticado via JWT + installation token, instalável por
repositório e revogável em um clique.

## Consequências
Exige lógica de renovação automática de installation token (ver
`internal/github`). Em troca, elimina dependência de conta pessoal e reduz
superfície de permissão ao mínimo necessário.

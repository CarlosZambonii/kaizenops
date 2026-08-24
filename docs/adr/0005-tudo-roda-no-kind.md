# ADR-5: Tudo roda no Kind, sem exceção

## Status
Aceito

## Contexto
O projeto é um laboratório de Kubernetes além de um sistema de SPC/ML.
Webhooks do GitHub Actions toleram latência de segundos, então não há
requisito de baixa latência que justifique infraestrutura cloud gerenciada.

## Decisão
Todo o ambiente roda em um cluster Kind local, provisionado via Terraform.
Nenhum componente roda fora do cluster em produção do projeto.

## Consequências
Maximiza valor de aprendizado em Kubernetes (NetworkPolicies, RBAC, HPA,
PodSecurityStandards). Exige Docker Compose (ver seção 4.5 do CLAUDE.md)
para ciclo de desenvolvimento rápido antes de subir no Kind.

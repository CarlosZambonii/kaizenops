# ADR-4: New Relic em vez de Grafana self-hosted

## Status
Aceito

## Contexto
É preciso uma camada de visualização e alerting sobre os dados de SPC/DORA.
Grafana self-hosted exigiria mais um componente stateful no cluster Kind.

## Decisão
Usar New Relic (custom events + APM) como camada de visualização e
alerting. Grafana fica documentado como plano B, não implementado a menos
que New Relic se mostre insuficiente.

## Consequências
Menos um componente para operar no cluster. Reaproveita instrumentação APM
já validada. Cria dependência de uma conta externa — mitigado pela ADR-3
(TimescaleDB continua sendo a fonte da verdade).

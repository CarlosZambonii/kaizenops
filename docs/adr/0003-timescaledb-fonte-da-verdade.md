# ADR-3: TimescaleDB como fonte da verdade

## Status
Aceito

## Contexto
O sistema precisa persistir séries temporais de execuções de pipeline de
forma durável e consultável, independente da camada de visualização.

## Decisão
TimescaleDB é o armazenamento primário de todos os dados brutos e derivados.
New Relic recebe apenas custom events para visualização e alerting (ver
ADR-4), nunca é a fonte de dados primária.

## Consequências
Se a conta ou o serviço do New Relic ficar indisponível, nenhum dado
histórico é perdido — apenas a camada de dashboard/alerting fica sem dados
novos até a reconexão.

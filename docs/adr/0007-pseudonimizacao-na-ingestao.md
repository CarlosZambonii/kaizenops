# ADR-7: Pseudonimização na ingestão, não no dashboard

## Status
Aceito

## Contexto
O KaizenOps analisa processo, não pessoas (ver seção 1 do CLAUDE.md). Se o
nome de usuário cru for persistido e só mascarado na visualização, qualquer
acesso direto ao banco ou vazamento de configuração expõe identidade.

## Decisão
O autor de cada execução é pseudonimizado (hash com salt) no ponto de
ingestão, no Collector. O username cru nunca é gravado no TimescaleDB nem
trafega além do processo de ingestão.

## Consequências
Nenhuma consulta ou dashboard pode reverter a pseudonimização — é
estrutural, não uma política de exibição. Debugging que precise de
identidade real exige acesso fora do sistema (ex: correlação manual com o
GitHub), nunca através do KaizenOps.

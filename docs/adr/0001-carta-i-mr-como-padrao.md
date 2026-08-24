# ADR-1: Carta I-MR como padrão de controle estatístico

## Status
Aceito

## Contexto
Builds de CI/CD chegam um a um, não em lotes ou subgrupos racionais. Cada
execução de pipeline é uma observação individual (n=1).

## Decisão
Usar carta Individuals / Moving Range (I-MR) como carta de controle padrão
para métricas de build. X-bar/R fica disponível apenas para métricas já
agregadas por dia, onde subgrupos fazem sentido.

## Consequências
Regras de Nelson e cálculo de limites de controle no motor SPC devem operar
sobre observações individuais e amplitude móvel, não sobre médias de
subgrupo.

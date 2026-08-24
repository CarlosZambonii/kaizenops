# ADR-2: SPC em Go, ML em Python

## Status
Aceito

## Contexto
O motor SPC é determinístico e matematicamente simples (médias, amplitudes,
regras de Nelson, Cp/Cpk). O componente de ML precisa de um ecossistema
maduro de modelos (Isolation Forest, gradient boosting).

## Decisão
Implementar o motor SPC em Go, no mesmo binário/processo que faz a ingestão,
evitando um hop de rede para cálculo determinístico. Implementar o serviço de
ML em Python com FastAPI e scikit-learn, exposto via HTTP.

## Consequências
O Go precisa de um client HTTP com circuit breaker para chamar o ML service
(ver `internal/mlclient`). O sistema deve continuar funcional apenas com SPC
se o ML service estiver indisponível.

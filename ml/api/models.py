"""Modelos Pydantic de request/response da API do ML service.

Apenas os shapes de dados trocados com o cliente. Nenhuma logica de
negocio aqui (isso fica em ml/api/inference.py).
"""

from __future__ import annotations

from pydantic import BaseModel, Field


class AnomalyRequest(BaseModel):
    """Request de /anomaly e /predict (mesmo shape nos dois endpoints)."""

    job_name: str
    duration_seconds: float
    queue_seconds: float
    started_at: str = Field(
        ..., description="Timestamp ISO 8601, ex: '2026-08-24T10:00:00Z'"
    )


class AnomalyResponse(BaseModel):
    """Resposta de /anomaly."""

    is_anomaly: bool
    score: float
    model_version: str


class PredictResponse(BaseModel):
    """Resposta de /predict.

    O classificador de risco de falha ainda nao existe (so e treinado
    quando ha volume de dados suficiente -- ver CLAUDE.md, secao 4.3).
    Por isso `available` e sempre False, `risk_score` e `model_version`
    sempre None, com HTTP 200 (nao e erro, e o comportamento esperado).
    """

    available: bool
    risk_score: float | None
    model_version: str | None

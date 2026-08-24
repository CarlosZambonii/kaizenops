"""API FastAPI do ML service (Fase 4).

Apenas orquestracao: parsing do request (Pydantic, ver ml/api/models.py),
chamada ao modulo de inferencia (ml/api/inference.py) e montagem da
resposta. Nenhuma logica de negocio aqui.

O modelo de anomalia e carregado uma vez no startup (lifespan) a partir
de ML_MODEL_PATH / ML_METADATA_PATH (env vars, com defaults para
ml/models/isolation_forest.joblib e ml/models/metadata.json). Se os
arquivos nao existirem, o app sobe normalmente com o modelo ausente
(None) -- /anomaly responde 503 nesse caso, mas a API nunca crasha na
inicializacao por falta de modelo (ver CLAUDE.md, secao 1).
"""

from __future__ import annotations

import os
import sys
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, HTTPException

# ml/ nao tem __init__.py (namespace package). Mesmo padrao usado em
# ml/training/train.py: garantir ml/ no sys.path para que `features`
# seja importavel como pacote de topo.
_ML_DIR = Path(__file__).resolve().parent.parent
if str(_ML_DIR) not in sys.path:
    sys.path.insert(0, str(_ML_DIR))

from features.features import JobRecord, build_features  # noqa: E402

from .inference import AnomalyModel
from .models import AnomalyRequest, AnomalyResponse, PredictResponse

DEFAULT_MODEL_PATH = str(_ML_DIR / "models" / "isolation_forest.joblib")
DEFAULT_METADATA_PATH = str(_ML_DIR / "models" / "metadata.json")


def _load_anomaly_model() -> AnomalyModel | None:
    model_path = os.environ.get("ML_MODEL_PATH", DEFAULT_MODEL_PATH)
    metadata_path = os.environ.get("ML_METADATA_PATH", DEFAULT_METADATA_PATH)
    return AnomalyModel.load(model_path, metadata_path)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    app.state.anomaly_model = _load_anomaly_model()
    yield


app = FastAPI(title="KaizenOps ML Service", lifespan=lifespan)


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/anomaly", response_model=AnomalyResponse)
def anomaly(request: AnomalyRequest) -> AnomalyResponse:
    model: AnomalyModel | None = app.state.anomaly_model
    if model is None:
        raise HTTPException(status_code=503, detail="anomaly model not loaded")

    record = JobRecord(
        job_name=request.job_name,
        duration_seconds=request.duration_seconds,
        queue_seconds=request.queue_seconds,
        started_at=request.started_at,
    )
    feature_row = build_features([record])[0]
    is_anomaly, score = model.score(feature_row)
    return AnomalyResponse(
        is_anomaly=is_anomaly, score=score, model_version=model.model_version
    )


@app.post("/predict", response_model=PredictResponse)
def predict(request: AnomalyRequest) -> PredictResponse:
    # Classificador de risco de falha ainda nao existe -- so e treinado
    # quando ha volume de dados suficiente, por decisao do projeto (ver
    # CLAUDE.md, secao 4.3/4.4). Resposta graceful, sempre HTTP 200: nao
    # e um erro, e o comportamento esperado enquanto o modelo nao existir.
    del request  # request nao e usado ainda; mantido pelo shape do contrato.
    return PredictResponse(available=False, risk_score=None, model_version=None)

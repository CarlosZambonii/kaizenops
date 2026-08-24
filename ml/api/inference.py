"""Carrega o modelo de anomalia (Isolation Forest) e roda a predicao.

Toda a logica de negocio da inferencia vive aqui: carregar modelo +
metadata do disco, extrair a matriz de features usada pelo modelo e
decidir is_anomaly/score. ml/api/main.py so orquestra (parsing de
request, chamada a este modulo, montagem da resposta).

Assumption: `model_version` exposto para o cliente e o `feature_version`
gravado no metadata.json (unico campo de "versao" que o contrato de
metadata do treino define -- ver ml/training/train.py). Se o significado
pretendido for outro (ex: combinar com dataset_hash/trained_at para
identificar o artefato treinado, nao so a versao das features), ajustar
aqui.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

import joblib

# ml/ nao tem __init__.py (namespace package). Mesmo padrao usado em
# ml/training/train.py: garantir ml/ no sys.path para que `features`
# seja importavel como pacote de topo, independente de como este modulo
# foi carregado (pytest, uvicorn via "ml.api.main", ou script direto).
_ML_DIR = Path(__file__).resolve().parent.parent
if str(_ML_DIR) not in sys.path:
    sys.path.insert(0, str(_ML_DIR))

from features.features import FeatureRow  # noqa: E402


class AnomalyModel:
    """Isolation Forest treinado + metadata, carregados do disco."""

    def __init__(self, model: object, metadata: dict) -> None:
        self._model = model
        self._metadata = metadata

    @property
    def model_version(self) -> str:
        return str(self._metadata["feature_version"])

    @classmethod
    def load(cls, model_path: str, metadata_path: str) -> "AnomalyModel | None":
        """Carrega modelo e metadata do disco.

        Retorna None se o arquivo do modelo ou do metadata nao existir --
        a API precisa subir mesmo sem modelo treinado (ver CLAUDE.md,
        secao 1: "o ML e enriquecimento, nunca ponto unico de falha").
        """
        model_file = Path(model_path)
        metadata_file = Path(metadata_path)
        if not model_file.exists() or not metadata_file.exists():
            return None

        model = joblib.load(model_file)
        with open(metadata_file) as f:
            metadata = json.load(f)
        return cls(model=model, metadata=metadata)

    def score(self, feature_row: FeatureRow) -> tuple[bool, float]:
        """Roda a predicao sobre uma linha de features.

        Retorna (is_anomaly, score). `score` vem de score_samples do
        IsolationForest (quanto mais negativo, mais anomalo); is_anomaly
        reflete a decisao do modelo (predict() == -1), coerente com a
        mesma definicao de contamination_rate usada no gate de treino.
        job_name nao entra na matriz -- mesmas 4 colunas numericas usadas
        no treino (ver ml/training/train.py:_feature_matrix).
        """
        features = [
            [
                feature_row.duration_seconds,
                feature_row.queue_seconds,
                feature_row.hour_of_day,
                feature_row.day_of_week,
            ]
        ]
        prediction = self._model.predict(features)[0]
        raw_score = self._model.score_samples(features)[0]
        return bool(prediction == -1), float(raw_score)

"""Treino do Isolation Forest sobre duracao de jobs.

Modelo prioritario da Fase 4 (ML Service): deteccao de anomalia nao
supervisionada, escolhida por funcionar com pouco dado. O treino aplica um
gate de qualidade -- um modelo que nao passa do gate nunca e publicado,
igual codigo quebrado nao passa do build (ver CLAUDE.md, secao 4.4).

Uso como script:

    python -m ml.training [caminho/para/dados.json]

Sem argumento, gera um dataset sintetico de demonstracao. O JSON de entrada
e uma lista de objetos com os campos de JobRecord:

    [{"job_name": "test", "duration_seconds": 120.0,
      "queue_seconds": 5.0, "started_at": "2026-08-24T10:00:00Z"}, ...]
"""

from __future__ import annotations

import hashlib
import json
import os
import random
import sys
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path

import joblib
import numpy as np
from sklearn.ensemble import IsolationForest

# ml/ nao tem __init__.py (namespace package) e este modulo precisa ser
# importavel tanto via pytest (rootdir insertion) quanto via execucao direta
# do script, independente do cwd. Garantir explicitamente que ml/ esta no
# sys.path evita depender desses detalhes de invocacao.
_ML_DIR = Path(__file__).resolve().parent.parent
if str(_ML_DIR) not in sys.path:
    sys.path.insert(0, str(_ML_DIR))

from features.features import (  # noqa: E402
    FEATURE_VERSION,
    FeatureRow,
    JobRecord,
    build_features,
)

# Gate de qualidade (ver CLAUDE.md, secao 4.4, item 3).
MIN_SAMPLES = 20
MIN_CONTAMINATION_RATE = 0.005
MAX_CONTAMINATION_RATE = 0.20

# Fracao de contaminacao passada ao IsolationForest. Fixa (nao "auto") para
# que o treino seja reprodutivel: o parametro define o quantil de score usado
# como limiar, entao a contamination_rate medida nos proprios dados de
# treino fica proxima deste valor por construcao.
_CONTAMINATION_PARAM = 0.05
_RANDOM_STATE = 42


@dataclass
class TrainResult:
    """Resultado de um treino que passou pelo gate de qualidade."""

    model: object  # sklearn.ensemble.IsolationForest treinado
    metadata: dict


def _dataset_hash(records: list[JobRecord]) -> str:
    """Hash sha256 de uma representacao estavel e serializavel dos records."""
    payload = [
        {
            "job_name": r.job_name,
            "duration_seconds": r.duration_seconds,
            "queue_seconds": r.queue_seconds,
            "started_at": r.started_at,
        }
        for r in records
    ]
    serialized = json.dumps(payload, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


def _feature_matrix(rows: list[FeatureRow]) -> np.ndarray:
    """Extrai as features numericas usadas pelo IsolationForest.

    job_name e categorico e nao entra na matriz: o modelo prioritario opera
    sobre duration_seconds, queue_seconds, hour_of_day e day_of_week.
    """
    return np.array(
        [
            [row.duration_seconds, row.queue_seconds, row.hour_of_day, row.day_of_week]
            for row in rows
        ],
        dtype=float,
    )


def train(records: list[JobRecord]) -> TrainResult:
    """Treina o Isolation Forest e aplica o gate de qualidade.

    Levanta ValueError (modelo nao publicavel) se:
      - n_samples < MIN_SAMPLES, ou
      - a contamination_rate medida nos dados de treino cair fora de
        [MIN_CONTAMINATION_RATE, MAX_CONTAMINATION_RATE].
    """
    n_samples = len(records)
    if n_samples < MIN_SAMPLES:
        raise ValueError(
            f"gate de qualidade: n_samples={n_samples} abaixo do minimo de "
            f"{MIN_SAMPLES}"
        )

    feature_rows = build_features(records)
    features = _feature_matrix(feature_rows)

    model = IsolationForest(
        contamination=_CONTAMINATION_PARAM, random_state=_RANDOM_STATE
    )
    model.fit(features)

    predictions = model.predict(features)
    contamination_rate = float(np.mean(predictions == -1))

    if not (MIN_CONTAMINATION_RATE <= contamination_rate <= MAX_CONTAMINATION_RATE):
        raise ValueError(
            "gate de qualidade: contamination_rate="
            f"{contamination_rate:.4f} fora do intervalo aceitavel "
            f"[{MIN_CONTAMINATION_RATE}, {MAX_CONTAMINATION_RATE}]"
        )

    metadata = {
        "trained_at": datetime.now(timezone.utc).isoformat(),
        "dataset_hash": _dataset_hash(records),
        "feature_version": FEATURE_VERSION,
        "n_samples": n_samples,
        "contamination_rate": contamination_rate,
    }

    return TrainResult(model=model, metadata=metadata)


def save_artifact(result: TrainResult, model_path: str, metadata_path: str) -> None:
    """Persiste o modelo (joblib) e o metadata (JSON) em disco."""
    joblib.dump(result.model, model_path)
    with open(metadata_path, "w") as f:
        json.dump(result.metadata, f, indent=2)


def _load_records_from_json(path: str) -> list[JobRecord]:
    with open(path) as f:
        data = json.load(f)
    return [
        JobRecord(
            job_name=item["job_name"],
            duration_seconds=float(item["duration_seconds"]),
            queue_seconds=float(item["queue_seconds"]),
            started_at=item["started_at"],
        )
        for item in data
    ]


def _synthetic_demo_records(n: int = 200) -> list[JobRecord]:
    """Dataset sintetico apenas para demonstrar o script sem dados reais."""
    rng = random.Random(42)
    base = datetime(2026, 8, 1, tzinfo=timezone.utc)
    records: list[JobRecord] = []
    for i in range(n):
        started_at = base + timedelta(hours=i)
        duration = rng.gauss(120.0, 15.0)
        if i % 40 == 0:  # poucos outliers, mantem contamination_rate no range
            duration *= 4
        records.append(
            JobRecord(
                job_name="unit-tests",
                duration_seconds=max(duration, 1.0),
                queue_seconds=max(rng.gauss(5.0, 2.0), 0.0),
                started_at=started_at.isoformat().replace("+00:00", "Z"),
            )
        )
    return records


def main(argv: list[str] | None = None) -> None:
    argv = sys.argv[1:] if argv is None else argv

    if argv:
        input_records = _load_records_from_json(argv[0])
    else:
        input_records = _synthetic_demo_records()

    train_result = train(input_records)

    models_dir = _ML_DIR / "models"
    os.makedirs(models_dir, exist_ok=True)
    model_path = str(models_dir / "isolation_forest.joblib")
    metadata_path = str(models_dir / "metadata.json")

    save_artifact(train_result, model_path, metadata_path)

    print(
        f"modelo treinado: n_samples={train_result.metadata['n_samples']} "
        f"contamination_rate={train_result.metadata['contamination_rate']:.4f}"
    )
    print(f"salvo em {model_path} e {metadata_path}")


if __name__ == "__main__":
    main()

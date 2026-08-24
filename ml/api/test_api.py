"""Testes da API FastAPI do ML service."""

from __future__ import annotations

import random
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

_ML_DIR = Path(__file__).resolve().parent.parent
if str(_ML_DIR) not in sys.path:
    sys.path.insert(0, str(_ML_DIR))

from features.features import JobRecord  # noqa: E402
from training.train import save_artifact, train  # noqa: E402

from api.main import app  # noqa: E402

ANOMALY_PAYLOAD = {
    "job_name": "unit-tests",
    "duration_seconds": 480.0,
    "queue_seconds": 5.0,
    "started_at": "2026-08-01T12:00:00Z",
}


def _small_normal_records(n: int = 30) -> list[JobRecord]:
    """Dataset pequeno, mas suficiente para passar o gate de qualidade
    do treino (ver ml/training/train.py: MIN_SAMPLES=20)."""
    rng = random.Random(11)
    base = datetime(2026, 8, 1, tzinfo=timezone.utc)
    records: list[JobRecord] = []
    for i in range(n):
        started_at = base + timedelta(hours=i)
        duration = rng.gauss(120.0, 15.0)
        if i % 10 == 0:  # poucos outliers, mantem contamination_rate no range
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


@pytest.fixture
def trained_model_paths(tmp_path: Path) -> tuple[str, str]:
    """Treina um Isolation Forest minusculo de verdade e salva em disco,
    para exercitar o carregamento real do modelo pela API (nao mock)."""
    result = train(_small_normal_records())
    model_path = tmp_path / "model.joblib"
    metadata_path = tmp_path / "metadata.json"
    save_artifact(result, str(model_path), str(metadata_path))
    return str(model_path), str(metadata_path)


def test_healthz_always_ok() -> None:
    with TestClient(app) as client:
        response = client.get("/healthz")

    assert response.status_code == 200


class TestAnomalyEndpoint:
    def test_returns_200_with_loaded_model(
        self, monkeypatch: pytest.MonkeyPatch, trained_model_paths: tuple[str, str]
    ) -> None:
        model_path, metadata_path = trained_model_paths
        monkeypatch.setenv("ML_MODEL_PATH", model_path)
        monkeypatch.setenv("ML_METADATA_PATH", metadata_path)

        with TestClient(app) as client:
            response = client.post("/anomaly", json=ANOMALY_PAYLOAD)

        assert response.status_code == 200
        body = response.json()
        assert isinstance(body["is_anomaly"], bool)
        assert isinstance(body["score"], float)
        assert body["model_version"] == "1.0.0"

    def test_returns_503_when_model_files_missing(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        monkeypatch.setenv("ML_MODEL_PATH", str(tmp_path / "missing.joblib"))
        monkeypatch.setenv("ML_METADATA_PATH", str(tmp_path / "missing.json"))

        with TestClient(app) as client:
            response = client.post("/anomaly", json=ANOMALY_PAYLOAD)

        assert response.status_code == 503
        assert "detail" in response.json()

    def test_rejects_malformed_request_with_422(
        self, monkeypatch: pytest.MonkeyPatch, trained_model_paths: tuple[str, str]
    ) -> None:
        model_path, metadata_path = trained_model_paths
        monkeypatch.setenv("ML_MODEL_PATH", model_path)
        monkeypatch.setenv("ML_METADATA_PATH", metadata_path)

        with TestClient(app) as client:
            response = client.post("/anomaly", json={"job_name": "unit-tests"})

        assert response.status_code == 422


class TestPredictEndpoint:
    def test_always_unavailable_with_model_loaded(
        self, monkeypatch: pytest.MonkeyPatch, trained_model_paths: tuple[str, str]
    ) -> None:
        model_path, metadata_path = trained_model_paths
        monkeypatch.setenv("ML_MODEL_PATH", model_path)
        monkeypatch.setenv("ML_METADATA_PATH", metadata_path)

        with TestClient(app) as client:
            response = client.post("/predict", json=ANOMALY_PAYLOAD)

        assert response.status_code == 200
        assert response.json() == {
            "available": False,
            "risk_score": None,
            "model_version": None,
        }

    def test_always_unavailable_without_anomaly_model(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        monkeypatch.setenv("ML_MODEL_PATH", str(tmp_path / "missing.joblib"))
        monkeypatch.setenv("ML_METADATA_PATH", str(tmp_path / "missing.json"))

        with TestClient(app) as client:
            response = client.post("/predict", json=ANOMALY_PAYLOAD)

        assert response.status_code == 200
        assert response.json()["available"] is False

"""Testes do treino do Isolation Forest e do gate de qualidade."""

from __future__ import annotations

import json
import random
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pytest
from sklearn.ensemble import IsolationForest

_ML_DIR = Path(__file__).resolve().parent.parent
if str(_ML_DIR) not in sys.path:
    sys.path.insert(0, str(_ML_DIR))

from features.features import FEATURE_VERSION, JobRecord  # noqa: E402
from training.train import (  # noqa: E402
    MAX_CONTAMINATION_RATE,
    MIN_CONTAMINATION_RATE,
    MIN_SAMPLES,
    TrainResult,
    save_artifact,
    train,
)


def _normal_records(n: int = 200) -> list[JobRecord]:
    """Dataset sintetico "normal": maioria dos pontos parecidos, poucos
    outliers claros, o suficiente para passar o gate de qualidade.
    """
    rng = random.Random(7)
    base = datetime(2026, 8, 1, tzinfo=timezone.utc)
    records: list[JobRecord] = []
    for i in range(n):
        started_at = base + timedelta(hours=i)
        duration = rng.gauss(120.0, 15.0)
        if i % 40 == 0:
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


def _too_few_records(n: int = 5) -> list[JobRecord]:
    """Dataset degenerado: abaixo do minimo de amostras exigido pelo gate."""
    base = datetime(2026, 8, 1, tzinfo=timezone.utc)
    return [
        JobRecord(
            job_name="unit-tests",
            duration_seconds=100.0 + i,
            queue_seconds=2.0,
            started_at=(base + timedelta(hours=i)).isoformat().replace(
                "+00:00", "Z"
            ),
        )
        for i in range(n)
    ]


def _identical_records(n: int = 30) -> list[JobRecord]:
    """Dataset degenerado: todos os valores identicos (variancia zero)."""
    started_at = "2026-08-01T10:00:00Z"
    return [
        JobRecord(
            job_name="unit-tests",
            duration_seconds=120.0,
            queue_seconds=5.0,
            started_at=started_at,
        )
        for _ in range(n)
    ]


class TestTrain:
    def test_normal_dataset_passes_gate(self) -> None:
        result = train(_normal_records())

        assert isinstance(result, TrainResult)
        assert isinstance(result.model, IsolationForest)
        assert result.metadata["n_samples"] == 200
        assert result.metadata["feature_version"] == FEATURE_VERSION
        assert (
            MIN_CONTAMINATION_RATE
            <= result.metadata["contamination_rate"]
            <= MAX_CONTAMINATION_RATE
        )

    def test_metadata_has_required_fields(self) -> None:
        result = train(_normal_records())
        metadata = result.metadata

        for field in (
            "trained_at",
            "dataset_hash",
            "feature_version",
            "n_samples",
            "contamination_rate",
        ):
            assert field in metadata

        # trained_at deve ser ISO8601 parseavel.
        datetime.fromisoformat(metadata["trained_at"])
        # dataset_hash deve ser um hex sha256 (64 chars).
        assert len(metadata["dataset_hash"]) == 64
        int(metadata["dataset_hash"], 16)  # nao levanta ValueError

    def test_dataset_hash_is_deterministic(self) -> None:
        records = _normal_records()
        result_a = train(records)
        result_b = train(records)

        assert result_a.metadata["dataset_hash"] == result_b.metadata["dataset_hash"]

    def test_dataset_hash_changes_with_data(self) -> None:
        records_a = _normal_records(n=200)
        records_b = _normal_records(n=200)
        records_b[0].duration_seconds += 1.0

        result_a = train(records_a)
        result_b = train(records_b)

        assert result_a.metadata["dataset_hash"] != result_b.metadata["dataset_hash"]

    def test_too_few_samples_raises_value_error(self) -> None:
        records = _too_few_records(n=MIN_SAMPLES - 1)

        with pytest.raises(ValueError):
            train(records)

    def test_exactly_min_samples_boundary_does_not_raise_for_sample_count(
        self,
    ) -> None:
        # No limite exato do gate de n_samples, o treino nao deve ser
        # rejeitado por causa da contagem de amostras (pode ainda ser
        # rejeitado pelo gate de contamination_rate, o que tambem e valido).
        records = _normal_records(n=MIN_SAMPLES)
        try:
            train(records)
        except ValueError as exc:
            assert "n_samples" not in str(exc)

    def test_identical_values_dataset_raises_value_error(self) -> None:
        with pytest.raises(ValueError):
            train(_identical_records())

    def test_empty_dataset_raises_value_error(self) -> None:
        with pytest.raises(ValueError):
            train([])


class TestSaveArtifact:
    def test_save_artifact_writes_model_and_metadata(self, tmp_path: Path) -> None:
        result = train(_normal_records())
        model_path = tmp_path / "model.joblib"
        metadata_path = tmp_path / "metadata.json"

        save_artifact(result, str(model_path), str(metadata_path))

        assert model_path.exists()
        assert metadata_path.exists()

        with open(metadata_path) as f:
            saved_metadata = json.load(f)
        assert saved_metadata == result.metadata

    def test_saved_model_can_be_loaded_and_used(self, tmp_path: Path) -> None:
        import joblib

        result = train(_normal_records())
        model_path = tmp_path / "model.joblib"
        metadata_path = tmp_path / "metadata.json"

        save_artifact(result, str(model_path), str(metadata_path))
        loaded_model = joblib.load(model_path)

        prediction = loaded_model.predict([[120.0, 5.0, 10, 0]])
        assert prediction[0] in (-1, 1)

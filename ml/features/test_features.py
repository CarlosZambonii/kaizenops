from datetime import datetime

from features import FEATURE_VERSION, FeatureRow, JobRecord, build_features


def test_feature_version_is_pinned() -> None:
    assert FEATURE_VERSION == "1.0.0"


def test_build_features_with_empty_list_returns_empty_list() -> None:
    assert build_features([]) == []


def test_build_features_extracts_hour_and_day_of_week() -> None:
    record = JobRecord(
        job_name="unit-tests",
        duration_seconds=42.5,
        queue_seconds=3.0,
        started_at="2026-08-24T10:00:00Z",
    )

    expected_weekday = datetime(2026, 8, 24).weekday()

    rows = build_features([record])

    assert rows == [
        FeatureRow(
            job_name="unit-tests",
            duration_seconds=42.5,
            queue_seconds=3.0,
            hour_of_day=10,
            day_of_week=expected_weekday,
        )
    ]


def test_build_features_preserves_order_and_handles_multiple_records() -> None:
    records = [
        JobRecord(
            job_name="build",
            duration_seconds=120.0,
            queue_seconds=5.0,
            started_at="2026-08-24T00:00:00Z",
        ),
        JobRecord(
            job_name="lint",
            duration_seconds=15.0,
            queue_seconds=1.0,
            started_at="2026-08-30T23:59:59Z",
        ),
    ]

    rows = build_features(records)

    assert [row.job_name for row in rows] == ["build", "lint"]
    assert rows[0].hour_of_day == 0
    assert rows[1].hour_of_day == 23
    # 2026-08-24 e uma segunda-feira (weekday 0), 2026-08-30 e um domingo (weekday 6).
    assert rows[0].day_of_week == 0
    assert rows[1].day_of_week == 6


def test_day_of_week_monday_is_zero_and_sunday_is_six() -> None:
    monday = JobRecord(
        job_name="job",
        duration_seconds=1.0,
        queue_seconds=0.0,
        started_at="2026-08-24T00:00:00Z",
    )
    sunday = JobRecord(
        job_name="job",
        duration_seconds=1.0,
        queue_seconds=0.0,
        started_at="2026-08-30T00:00:00Z",
    )

    rows = build_features([monday, sunday])

    assert rows[0].day_of_week == 0
    assert rows[1].day_of_week == 6


def test_build_features_is_pure_and_deterministic() -> None:
    records = [
        JobRecord(
            job_name="deploy",
            duration_seconds=300.0,
            queue_seconds=10.0,
            started_at="2026-01-05T14:30:00Z",
        )
    ]

    first_call = build_features(records)
    second_call = build_features(records)

    assert first_call == second_call
    # A lista de entrada nao deve ser mutada por build_features.
    assert records == [
        JobRecord(
            job_name="deploy",
            duration_seconds=300.0,
            queue_seconds=10.0,
            started_at="2026-01-05T14:30:00Z",
        )
    ]


def test_build_features_accepts_offset_instead_of_z_suffix() -> None:
    record = JobRecord(
        job_name="job",
        duration_seconds=1.0,
        queue_seconds=0.0,
        started_at="2026-08-24T10:00:00+00:00",
    )

    rows = build_features([record])

    assert rows[0].hour_of_day == 10

"""Feature engineering compartilhado entre treino e inferencia.

Este modulo nao faz IO. Ele apenas transforma registros de execucao de job
(JobRecord) em linhas de features (FeatureRow), tanto para o treino dos
modelos quanto para a inferencia em tempo real no servico FastAPI. Usar o
mesmo codigo nos dois pontos evita training-serving skew.

FEATURE_VERSION deve ser incrementado sempre que a logica de
`build_features` mudar de forma que afete o formato ou o significado das
features geradas. O valor e gravado no metadata JSON de cada modelo
treinado (ver docs da Fase 4 / ml/training).
"""

from dataclasses import dataclass
from datetime import datetime

FEATURE_VERSION = "1.0.0"


@dataclass
class JobRecord:
    """Um registro bruto de execucao de job, vindo do TimescaleDB."""

    job_name: str
    duration_seconds: float
    queue_seconds: float
    started_at: str  # ISO 8601, ex "2026-08-24T10:00:00Z"


@dataclass
class FeatureRow:
    """Uma linha de features derivada de um JobRecord."""

    job_name: str
    duration_seconds: float
    queue_seconds: float
    hour_of_day: int  # 0-23, extraido de started_at
    day_of_week: int  # 0=segunda .. 6=domingo, extraido de started_at


def _parse_started_at(started_at: str) -> datetime:
    """Faz o parse de um timestamp ISO 8601, aceitando o sufixo 'Z'."""
    normalized = started_at.replace("Z", "+00:00")
    return datetime.fromisoformat(normalized)


def build_features(records: list[JobRecord]) -> list[FeatureRow]:
    """Transforma registros brutos de job em linhas de features.

    Pura e deterministica: mesma entrada sempre produz a mesma saida, sem
    nenhum acesso a IO (banco, rede, relogio do sistema). Lista vazia
    retorna lista vazia, sem erro.
    """
    rows: list[FeatureRow] = []
    for record in records:
        started_at = _parse_started_at(record.started_at)
        rows.append(
            FeatureRow(
                job_name=record.job_name,
                duration_seconds=record.duration_seconds,
                queue_seconds=record.queue_seconds,
                hour_of_day=started_at.hour,
                # datetime.weekday(): 0=segunda .. 6=domingo, mesma convencao do contrato.
                day_of_week=started_at.weekday(),
            )
        )
    return rows

"""Treino do modelo prioritario da Fase 4: Isolation Forest nao supervisionado
sobre duracao de jobs, com gate de qualidade.
"""

from .train import TrainResult, main, save_artifact, train

__all__ = ["TrainResult", "main", "save_artifact", "train"]

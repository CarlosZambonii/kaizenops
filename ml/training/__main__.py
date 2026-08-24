"""Permite `python -m ml.training` (em vez de `python -m ml.training.train`,
que dispara um RuntimeWarning de dupla importação porque
ml/training/__init__.py já reexporta o submódulo train).
"""

from .train import main

if __name__ == "__main__":
    main()

# syntax=docker/dockerfile:1
#
# Build context MUST be the repository root:
#   docker build -f ml/api/Dockerfile .
#
# This is required because ml.api imports ml.features / ml.training as
# sibling packages of the same "ml" top-level package.

FROM python:3.12-slim AS base

# Pull in Debian's own security patches for the base image's OS packages
# before anything else — several CVEs Trivy flags against python:3.12-slim
# already have a fix in the Debian repo, we just need to pull it in.
RUN apt-get update && apt-get upgrade -y && rm -rf /var/lib/apt/lists/*

# Install dependencies as root first, in their own cached layer.
WORKDIR /app
COPY ml/requirements.txt ml/requirements.txt
RUN pip install --no-cache-dir -r ml/requirements.txt

# Copy the rest of the ml/ package tree.
COPY ml/ ml/

# Non-root user for runtime.
RUN groupadd --system app && useradd --system --gid app --no-create-home app
USER app

EXPOSE 8000
CMD ["uvicorn", "ml.api.main:app", "--host", "0.0.0.0", "--port", "8000"]

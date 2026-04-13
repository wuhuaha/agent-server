FROM python:3.11-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    MODELSCOPE_CACHE=/models/modelscope \
    HF_HOME=/models/hf \
    TORCH_HOME=/models/torch \
    AGENT_SERVER_FUNASR_HOST=0.0.0.0 \
    AGENT_SERVER_FUNASR_PORT=8091 \
    AGENT_SERVER_FUNASR_DEVICE=cpu \
    AGENT_SERVER_FUNASR_MODEL=iic/SenseVoiceSmall \
    AGENT_SERVER_FUNASR_LANGUAGE=auto \
    AGENT_SERVER_FUNASR_DISABLE_UPDATE=true \
    AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE=false \
    AGENT_SERVER_FUNASR_USE_ITN=true \
    AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER=energy

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libsndfile1 \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --upgrade \
    pip \
    "setuptools<82" \
    wheel \
    "hatchling>=1.25.0" \
    "editables>=0.5"

ARG TORCH_INDEX_URL=https://download.pytorch.org/whl/cpu
ARG TORCH_VERSION=2.11.0
ARG TORCHAUDIO_VERSION=2.11.0

RUN python -m pip install --index-url "${TORCH_INDEX_URL}" \
    "torch==${TORCH_VERSION}" \
    "torchaudio==${TORCHAUDIO_VERSION}"

COPY workers/python /app/workers/python

RUN python -m pip install --no-build-isolation -e "/app/workers/python[runtime,stream-vad]"

VOLUME ["/models/modelscope", "/models/hf", "/models/torch"]
EXPOSE 8091

CMD ["python", "-m", "agent_server_workers.funasr_service", "--host", "0.0.0.0", "--port", "8091", "--device", "cpu"]


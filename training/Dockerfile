FROM cgr.dev/chainguard/python@sha256:125969103add9ace8bdbad31acbb07d2e5740e065312688534f0c666a181b987 AS dependencies
USER 0:0
COPY training/requirements.lock /tmp/requirements.lock
COPY wheelhouse/ /tmp/wheelhouse/
RUN ["/usr/bin/python", "-I", "-m", "venv", "/opt/venv"]
RUN ["/opt/venv/bin/python", "-I", "-m", "pip", "install", "--no-index", "--no-cache-dir", "--find-links=/tmp/wheelhouse", "--require-hashes", "-r", "/tmp/requirements.lock"]

FROM cgr.dev/chainguard/python@sha256:125969103add9ace8bdbad31acbb07d2e5740e065312688534f0c666a181b987
COPY --from=dependencies /opt/venv /opt/venv
COPY --chmod=0444 training/train_mlp.py /opt/cpu03/train_mlp.py
COPY --chmod=0444 training/runtime-lock.json training/requirements.lock training/wheelhouse.sha256 training/runtime-packages.lock.json /opt/cpu03/materials/
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OMP_NUM_THREADS=1 \
    MKL_NUM_THREADS=1 \
    CUDA_VISIBLE_DEVICES="" \
    WORLD_SIZE=1
WORKDIR /opt/cpu03
USER 10001:10001
ENTRYPOINT ["/opt/venv/bin/python", "-I", "/opt/cpu03/train_mlp.py"]

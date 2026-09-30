FROM docker.io/library/python@sha256:7c61056e61ac89e852de05f3dc6fa51a6dd2181797bceed46aa725dd7cb2cd3b AS dependencies
COPY training/requirements.lock /tmp/requirements.lock
COPY wheelhouse/ /tmp/wheelhouse/
RUN python -m venv /opt/venv \
    && /opt/venv/bin/python -m pip install --no-index --no-cache-dir \
       --find-links=/tmp/wheelhouse --require-hashes -r /tmp/requirements.lock

FROM docker.io/library/python@sha256:7c61056e61ac89e852de05f3dc6fa51a6dd2181797bceed46aa725dd7cb2cd3b
COPY system-packages/ /tmp/system-packages/
RUN dpkg -i /tmp/system-packages/*.deb \
    && dpkg-query -W libssl3t64 openssl openssl-provider-legacy \
    && rm -rf /tmp/system-packages
COPY --from=dependencies /opt/venv /opt/venv
COPY --chmod=0444 training/train_mlp.py /opt/cpu03/train_mlp.py
COPY --chmod=0444 training/runtime-lock.json training/requirements.lock training/wheelhouse.sha256 training/system-packages.sha256 /opt/cpu03/materials/
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OMP_NUM_THREADS=1 \
    MKL_NUM_THREADS=1 \
    CUDA_VISIBLE_DEVICES="" \
    WORLD_SIZE=1
WORKDIR /opt/cpu03
USER 10001:10001
ENTRYPOINT ["/opt/venv/bin/python", "-I", "/opt/cpu03/train_mlp.py"]

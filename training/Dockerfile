FROM docker.io/library/python@sha256:ac76900038d8606cc99b413d4ede77bc7152f1e42b94cf5d50d4b80a999652fe AS dependencies
COPY training/requirements.lock /tmp/requirements.lock
COPY wheelhouse/ /tmp/wheelhouse/
RUN python -m venv /opt/venv \
    && /opt/venv/bin/python -m pip install --no-index --no-cache-dir \
       --find-links=/tmp/wheelhouse --require-hashes -r /tmp/requirements.lock

FROM docker.io/library/python@sha256:ac76900038d8606cc99b413d4ede77bc7152f1e42b94cf5d50d4b80a999652fe
COPY --from=dependencies /opt/venv /opt/venv
COPY training/train_mlp.py /opt/cpu03/train_mlp.py
COPY training/runtime-lock.json training/requirements.lock training/wheelhouse.sha256 /opt/cpu03/materials/
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OMP_NUM_THREADS=1 \
    MKL_NUM_THREADS=1 \
    CUDA_VISIBLE_DEVICES="" \
    WORLD_SIZE=1
WORKDIR /opt/cpu03
USER 10001:10001
ENTRYPOINT ["/opt/venv/bin/python", "-I", "/opt/cpu03/train_mlp.py"]

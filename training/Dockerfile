FROM cgr.dev/chainguard/python@sha256:125969103add9ace8bdbad31acbb07d2e5740e065312688534f0c666a181b987 AS dependencies
USER 0:0
COPY training/requirements.lock /tmp/requirements.lock
COPY wheelhouse/ /tmp/wheelhouse/
RUN ["/usr/bin/python", "-I", "-m", "venv", "/opt/venv"]
RUN ["/opt/venv/bin/python", "-I", "-m", "pip", "install", "--no-index", "--no-cache-dir", "--find-links=/tmp/wheelhouse", "--require-hashes", "-r", "/tmp/requirements.lock"]

FROM cgr.dev/chainguard/python@sha256:125969103add9ace8bdbad31acbb07d2e5740e065312688534f0c666a181b987
USER 0:0
RUN ["/usr/bin/python", "-I", "-c", "from pathlib import Path; p=Path('/etc/passwd'); g=Path('/etc/group'); users=p.read_bytes(); groups=g.read_bytes(); assert not any(r.split(b':')[0]==b'modeldev' or int(r.split(b':')[2])==10001 for r in users.splitlines() if r and not r.startswith(b'#')), 'modeldev username or UID already exists'; assert not any(r.split(b':')[0]==b'modeldev' or int(r.split(b':')[2])==10001 for r in groups.splitlines() if r and not r.startswith(b'#')), 'modeldev group or GID already exists'; p.write_bytes(users+(b'' if not users or users.endswith(b'\\n') else b'\\n')+b'modeldev:x:10001:10001:ModelDev training:/nonexistent:/sbin/nologin\\n'); g.write_bytes(groups+(b'' if not groups or groups.endswith(b'\\n') else b'\\n')+b'modeldev:x:10001:\\n')"]
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

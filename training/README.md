# CPU03 training workload

Source task: `ANI-doc/02-issues/modeldev/cpu-p01/tasks/CPU03.md`.
The fixed CSV and MLP recipe originate in that package's
`reference/v04/cpu-baseline/`; historical package reports are not current evidence.

## Workload contract

The command requires `--data`, `--output`, `--expected-input-sha256`, and
`--expected-input-bytes`. Before importing PyTorch or creating output, it checks
the selected regular file's size and SHA256, all 1024 rows with 16 finite float32
features and binary labels, single-process WORLD_SIZE, and paths without symlinks.
Nonempty or non-directory output paths are refused and preserved; an existing
empty directory is accepted.

The CPU MLP is 16→32→2, with three epochs, batch size 64 and 48 optimizer steps.
`--learning-rate` accepts a decimal in (0, 0.1], at most 32 characters without
exponent notation, and defaults to 0.01. Its canonical value is recorded in the
summary and used by Adam. Tests compare actual checkpoint tensors to establish
that changing the learning rate affects computation.

Three registered internal recipes use this same workload:

- `success`: complete 48 steps and emit candidate files and their digests.
- `fail`: complete five optimizer updates, exit with `CPU03_RECIPE_FAILURE`,
  and retain only metrics.
- `slow-stop`: perform real optimization with a fixed 0.2-second delay after each
  update and a 30-second POSIX alarm. SIGTERM exits 143 while closing open files.

These recipes are internal acceptance commands, not ordinary user parameters or
a second training API. Callers cannot supply custom failure steps or time budgets.
`result.json` is a candidate result, not proof of S3 publication or business success.
The separate CPU08/CPU09 verifier must obtain bytes through BFF-authorized S3
access without mounting the original training PVC.

## Locked image materials

`runtime-lock.json` pins the linux/amd64 Chainguard Python manifest
`sha256:125969103add9ace8bdbad31acbb07d2e5740e065312688534f0c666a181b987`
in both Dockerfile stages. Builds never resolve a discovery tag. The base contains
Python 3.14.7 with the ordinary cp314 ABI; PyTorch remains 2.10.0+cpu. Of the ten
locked wheels, only Torch and MarkupSafe changed from cp313 to cp314; dependency
versions remained unchanged by that ABI migration.

`requirements.lock` pins versions and SHA256, `wheelhouse.sha256` fixes filenames,
and `runtime-packages.lock.json` records all 31 APK package names and versions,
including the legacy OpenSSL provider. The package database is retained. Torch
uses its wheel's bundled libgomp and the image's libstdc++; no host library is
copied. Optional NumPy integration emits a warning because this workload uses
tensors directly without NumPy.

The base has no shell. Exec-form Python commands create `/opt/venv` and install
the verified wheels offline during the build. The final image uses UID/GID
10001:10001, read-only program files, and the workload itself as PID 1. It contains
no selected dataset, checkpoint, or credentials and installs nothing at startup.

## Fedora execution

All generation, builds, training, reloads and scanning run on Fedora against a
fixed committed source. After selecting the task-owned Podman storage containing
the qualified base digest, use a fresh output directory:

```sh
bash training/build-image.sh FULL_SOURCE_SHA \
  /home/chabking/workspace/cpu-p01-20260930-01/cpu03/qualification-1259691-04/wheelhouse \
  /home/chabking/workspace/cpu-p01-20260930-01/cpu03/build-NEW_ATTEMPT
python3 training/tests/image_smoke.py "$(cat /home/chabking/workspace/cpu-p01-20260930-01/cpu03/build-NEW_ATTEMPT/image.id)" \
  /home/chabking/workspace/cpu-p01-20260930-01/cpu03/smoke-NEW_ATTEMPT
```

The build script rejects existing output directories, checks the source revision,
archives that Git tree, and copies only hash-verified locked wheels. It uses no
network, proxy forwarding or image pulls, is limited to two CPUs / 2 GiB, and
records the source, command, exits and local image identity. It does not push images.

The smoke tests actual nonroot PID 1 behavior without an init wrapper. Containers
are offline and read-only except isolated output/tmpfs, limited to two CPUs /
2 GiB and bounded to 90 seconds. A separate container generates the selected
fixture. Success checks actual file bytes and digests and independently reloads
the checkpoint with `weights_only=True`; fail and stop checks observe real
optimization first. Stop cleanup targets only the returned container ID after
retaining evidence. The image does not contain the test sources. To run the six
unchanged module tests inside it, the verification runner mounts the fixed source
read-only at `/source/training` and invokes `/opt/venv/bin/python -I -m unittest
discover -s /source/training/tests -p test_training.py -v`, with a separate bounded
writable temporary directory and the runner's resource limits.

## Fixed verification result

On Fedora, source `4d56b93b045233c1325a362fe89d506210223d47` produced local image
`sha256:0972fac83ba343e4d24e22bc35198bddf5c3e9d5c8ab3337694395beead3bd04`.
The run on 2026-09-30 at 15:17:51–15:21:59 UTC passed offline build, all six module
tests (38.123 seconds), and complete image smoke. Smoke covered the complete APK
lock, normal ABI and venv, dependency imports, success48, independent reload,
failure after five steps, and SIGTERM exit 143 after three observed steps.

Syft 1.52.0 recorded 69 artifacts: 31 APK, 14 binary and 24 Python records.
Grype 0.119.0 with the validated v6.1.9 database built at
`2026-09-30T06:32:47Z` passed the original `--fail-on high` gate: zero Critical,
zero High, eight Medium and two Low findings. All findings remain in the raw
report. There were no path excludes, VEX documents, only-fixed filtering or
user-added ignore rules. The four built-in RPM/DEB kernel-header rules matched
the previous scan configuration; this image has no RPM/DEB artifacts and no
ignored matches. This result does not mean the image has no vulnerabilities.

The OCI archive SHA256 is
`be102bd1fe66aadee64e782decf08c97dc07c5d1337ee3563bc8f8eba40bca6b`.
It identifies archive bytes, not a registry manifest digest. The run evidence is
indexed in `.scratch/cpu-p01/runs/20260930-01/cpu03/remediation-4d56b93-01/result.md`;
large OCI/checkpoint files remain in the corresponding Fedora task directory.
Later documentation or Go commits do not change the source identity of this image.

Earlier failed packaging, PID 1 and scan attempts remain separate in Git and the
CPU03 run records. In particular, `472c080` produced a different image whose scan
failed with 59 High findings; the new result does not relabel that old scan.

Signature trust-chain verification and registry publication remain unverified.
The consumed ENV handoff supplies no authorized registry destination. Target
cluster Trainer/KFP/BFF/S3 execution, L1–L4 and business acceptance are not proven
by this Fedora image run. Repository-wide gates and independent code review are
tracked separately from these workload results.

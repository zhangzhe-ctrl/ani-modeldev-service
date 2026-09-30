# CPU03 training workload

Source task: `ANI-doc/02-issues/modeldev/cpu-p01/tasks/CPU03.md`.
Fixture generation and the fixed MLP recipe originate in that package's
`reference/v04/cpu-baseline/`; its historical validation is not current evidence.

The first agreed test seam is the workload command plus its actual files. The
test uses a selected CSV, executes training in a subprocess, checks 48 metrics
and candidate file digests, and reloads the checkpoint with `weights_only=True`
in another process that imports no trainer implementation.

Run only on Fedora, against a fixed source commit with a CPU PyTorch environment:

```sh
python -m unittest discover -s training/tests -p 'test_training.py' -v
```

The first RED was observed on Fedora at source commit
`452fff617856bb4d1a3ccdb029e24cbf71aae093`: one failing behavior test, exit 1,
`CPU03_NOT_IMPLEMENTED`, after the real CPU PyTorch preflight succeeded. The
first implementation passed the same test on Fedora at
`b69667f242898ecef10cfe9481a954c91d6218e9`: one test, exit 0, including the
independent checkpoint reload. A missing PyTorch dependency is
`ENVIRONMENT_NOT_READY`, not a behavior RED.

The command requires `--data`, `--output`, `--expected-input-sha256`, and
`--expected-input-bytes`. Before importing PyTorch or creating output, the
admission check verifies a regular file's actual size and SHA256, all 1024 rows
and 16 finite float32 features, binary labels, single-process WORLD_SIZE, and
paths without symlinks. Nonempty and non-directory output paths are refused and
preserved; a real existing empty directory is accepted.
It must not be deployed until the remaining recipe tests and live integration
are implemented and checked. The first slice fixes three epochs, batch
size 64, and CPU MLP 16→32→2. `result.json` is a workload output candidate, not
proof of S3 publication or business success.

`runtime-lock.json` records the actual Python 3.13.11 container image identity
and PyTorch 2.10.0+cpu environment inspected on Fedora. `requirements.lock` pins
the resolved ten dependency wheels by version and SHA256; `wheelhouse.sha256`
records the filenames. A Fedora offline `pip install --dry-run --ignore-installed
--require-hashes` resolved this lock successfully. These files do not establish a
built workload image digest. Fixture CSVs, wheels, and model output stay out of
Git. PyTorch's optional NumPy integration reports a warning because this workload
uses tensors directly and does not install NumPy.

The next test is one parameterized admission behavior: reject mismatched input
identity, malformed fixed CSV (shape, labels, NaN/Inf), non-singleton WORLD_SIZE,
symlink input/output, and nonempty output before emitting optimization metrics or
creating output artifacts. At `fd9015bdea0af35c6c0b9d0898ae3a958c63e741`, Fedora
recorded the normal case passing and 14 rejection subcases failing in 83.035
seconds, exit 1. At `23aa203b24d3b60a4ab74d8686d36819eb2a76da`, the corresponding
checks passed both tests (including all 19 refusal scenarios) in 17.187 seconds,
exit 0.

The next behavior tests cover canonical `--learning-rate` in (0, 0.1] and an
existing empty output directory. The valid test runs both default 0.01 and explicit
0.02, requires the effective decimal in the summary, reloads both checkpoints,
and compares actual tensors to prove the selected rate affected training. Another
test rejects malformed/out-of-range/non-finite rates before output creation.
At `b17b0074314c5defba4031556ecf13b58769c79b`, Fedora observed the expected RED:
four tests in 29.814 seconds, one failure because the command rejected the valid
`--learning-rate 0.02` argument. The implementation now parses a maximum
32-character, non-exponent decimal in (0, 0.1], records its canonical decimal in
the summary, passes the value to Adam, and accepts an existing empty directory.
At `080320ea314e8d1ce885b18a1d488daabdc574b9`, all four module tests passed on
Fedora in 42.886 seconds, exit 0; the selected learning rate produced different
actual checkpoint tensors. Epochs and batch size remain fixed at 3 and 64.

The recipe tests cover the internal managed recipes required by the original
CPU03 task card, execution details 5–6. `--recipe fail` completes five real
optimizer steps, then exits with `CPU03_RECIPE_FAILURE`, retaining only metrics.
`--recipe slow-stop` keeps the normal 48-step computation, with a fixed 0.2-second
delay after each real step and a 30-second total budget. It reports these limits
as an `ani.cpu03.recipe.v1` stdout event. The test observes three actual optimizer
events before SIGTERM, requires exit within ten seconds, and rejects successful
checkpoint/candidate files. Test polling is bounded and targets only its own
subprocess. At `a3e321fbed242e6d64919a5a8db48a8d44edbf04`, both targeted tests
failed on Fedora in 6.466 seconds, exit 1: valid inputs passed dependency setup,
then the command rejected the absent `--recipe` argument before computation.
The implementation now accepts the three registered recipes, with `success` as
the default. Failure is raised after the fifth actual optimizer update and
flushed metric. Slow-stop uses a fixed POSIX alarm beginning after input admission
and before PyTorch import; its delay follows each real optimizer update. SIGTERM
keeps its normal process termination semantics. All six module tests passed at
`d212f25e2180413a3e538370f1b4a011a6fadba7` on Fedora in 55.861 seconds, exit 0.
This signal result applies to the workload subprocess; the packaged image's
actual entrypoint and PID 1 still require the separate image smoke below.

These recipe names select registered internal test commands. They are not
ordinary user parameters, free-form command options, or a second training API.
No caller may supply a custom failure step, sleep duration, or total-step budget.
The initial subprocess reload is CPU03 module evidence; the
independent BFF/S3 verifier belongs to CPU08/CPU09 and must not mount the original
training PVC.

## Offline image build and smoke

`Dockerfile` fixes the actual Python base digest in both stages, installs the ten
hash-pinned CPU wheels during the build without network access, and copies the
resulting environment into a runtime image. Its default user is `10001:10001`;
the entrypoint is the workload itself, with JSON metrics on stdout. It contains
no selected dataset, checkpoint, or registry credentials. Its entrypoint performs
no package installation or dependency download.

On Fedora only, after these files are committed and the exact commit is checked
out, use:

```sh
bash training/build-image.sh FULL_SOURCE_SHA \
  /home/chabking/workspace/cpu-p01-20260930-01/cpu03/wheelhouse \
  /home/chabking/workspace/cpu-p01-20260930-01/cpu03/build-NEW_ATTEMPT
python3 training/tests/image_smoke.py "$(cat /home/chabking/workspace/cpu-p01-20260930-01/cpu03/build-NEW_ATTEMPT/image.id)" \
  /home/chabking/workspace/cpu-p01-20260930-01/cpu03/smoke-NEW_ATTEMPT
```

The build script rejects an existing run directory, checks the current source
revision, archives that immutable Git tree, and copies only wheels named in the
lock before verifying their hashes. The build has no network or proxy forwarding,
uses at most two CPUs / 2 GiB, and records the command, source, exits and local image
identity. It neither downloads dependencies nor pushes an image.

The image smoke runs the real packaged entrypoint under its declared nonroot
user. It generates the selected fixture in a separate bounded Fedora container,
then binds that input read-only and gives each recipe its own output directory.
It checks success files and hashes, independently reloads the checkpoint in a new
container, observes five real steps before explicit failure, and sends SIGTERM to
the slow container only after observing real optimization. Every container is
offline, read-only except its output/tmpfs, limited to two CPUs / 2 GiB, and has a
90-second outer lifetime bound. The slow container is removed only by its exact
returned ID after retaining evidence and output. The smoke deliberately tests the
image's actual PID 1 behavior without adding an init wrapper.

The first build at `a9b3a67d1fa9b4651add4f7c36237e36b6d1cdc8` verified all ten
wheel hashes, then exited 125 before Dockerfile execution because Podman's build
command requires `--pull=never` for this optional-value flag. The local script is
corrected. The subsequent offline build passed at
`c3916acfea616e44c5a01f14b8d27adc493c44ea`, producing local image
`sha256:e89b63fcf5851dcc027a32b087409badbc813257c6ad72d69761275d9a0afadd`.
Its first image smoke passed the nonroot Python/PyTorch preflight and fixture
generation, then failed: the default entrypoint could not read the root-owned
mode-0600 trainer file. This is an image packaging RED before training; the
failure and PID 1 stop recipes were not reached. The Dockerfile now gives the
packaged trainer and material records explicit read-only mode 0444, independent
of the build-context umask. The new fixed image build and smoke remain pending.
The existing CI workflow has no
image-publish job; the consumed ENV handoff is still a NOT_RUN template without a
registry reference. Registry push therefore remains NOT_RUN until an explicit
authorized destination is available. A local image ID is not a registry digest,
and neither is target-cluster Trainer evidence.

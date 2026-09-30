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
paths without symlinks. Existing output paths are refused and preserved.
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
These new tests are not run; production still uses 0.01 and refuses existing
output paths until this RED/GREEN cycle completes. Epochs and batch size remain
fixed at 3 and 64. Controlled real-compute failure/stop recipes follow separately.
The initial subprocess reload is CPU03 module evidence; the
independent BFF/S3 verifier belongs to CPU08/CPU09 and must not mount the original
training PVC.

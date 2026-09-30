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
first implementation now performs the fixed training loop; its GREEN still
requires committing this source and running that commit on Fedora. A missing
PyTorch dependency is `ENVIRONMENT_NOT_READY`, not a behavior RED.

The command requires `--data`, `--output`, `--expected-input-sha256`, and
`--expected-input-bytes`. This partial implementation accepts the expected
identity arguments but does not yet reject a mismatch: that is the next behavior
cycle. It must not be deployed until input validation and controlled recipe
tests are implemented and checked. The first slice fixes three epochs, batch
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

After the first RED/GREEN cycle, add separate behavior cycles for incorrect input
digest/size, nonempty output refusal, malformed CSV, and controlled real-compute
failure/stop recipes. The initial subprocess reload is CPU03 module evidence;
the independent BFF/S3 verifier belongs to CPU08/CPU09 and must not mount the
original training PVC.

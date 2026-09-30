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

Current state: test and command seam only; `train_mlp.py` explicitly exits with
`CPU03_NOT_IMPLEMENTED`. The expected RED is the nonzero trainer exit for a valid
fixed CSV. A missing PyTorch dependency is `ENVIRONMENT_NOT_READY`, not that RED.
No test has been executed for this source yet.

The command requires `--data`, `--output`, `--expected-input-sha256`, and
`--expected-input-bytes`. The first slice fixes three epochs, batch size 64,
single-process CPU MLP 16→32→2. `result.json` is a workload output candidate,
not proof of S3 publication or business success.

After the first RED/GREEN cycle, add separate behavior cycles for incorrect input
digest/size, nonempty output refusal, malformed CSV, and controlled real-compute
failure/stop recipes. The initial subprocess reload is CPU03 module evidence;
the independent BFF/S3 verifier belongs to CPU08/CPU09 and must not mount the
original training PVC.

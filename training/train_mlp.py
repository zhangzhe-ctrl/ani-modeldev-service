"""Train the fixed CPU-P01 MLP from a selected CSV.

The result is a workload candidate. This command has no control-plane clients
and does not mark an Execution successful or publish files to object storage.
"""

import argparse
import csv
from datetime import datetime, timezone
from decimal import Decimal
import hashlib
import io
import json
import math
import os
from pathlib import Path
import re
import signal
import stat
import sys
import time


def checked_path(value):
    path = Path(os.path.abspath(value))
    for component in (*reversed(path.parents), path):
        if component.is_symlink():
            raise ValueError("symbolic links are not allowed in workload paths")
    return path


def learning_rate(value):
    if len(value) > 32 or not re.fullmatch(r"(0|[1-9][0-9]*)(\.[0-9]+)?", value):
        raise ValueError("learning rate must be a non-exponent decimal with at most 32 characters")
    decimal = Decimal(value)
    if not 0 < decimal <= Decimal("0.1"):
        raise ValueError("learning rate must be in (0, 0.1]")
    canonical = format(decimal, "f").rstrip("0").rstrip(".")
    return canonical, float(decimal)


def check_output(path):
    if path.exists() and (not path.is_dir() or any(path.iterdir())):
        raise FileExistsError("refusing to overwrite a nonempty or non-directory output path")


def load_selected_input(path, expected_sha256, expected_bytes):
    if not re.fullmatch(r"[0-9a-f]{64}", expected_sha256):
        raise ValueError("expected input SHA256 must be 64 lowercase hexadecimal characters")
    if not 0 < expected_bytes <= 32 * 1024 * 1024:
        raise ValueError("expected input size must be between 1 byte and 32 MiB")
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        identity = os.fstat(stream.fileno())
        if not stat.S_ISREG(identity.st_mode):
            raise ValueError("selected input must be a regular file")
        if identity.st_size != expected_bytes:
            raise ValueError("selected input size does not match the frozen input")
        contents = stream.read(expected_bytes + 1)
    digest = hashlib.sha256(contents).hexdigest()
    if len(contents) != expected_bytes or digest != expected_sha256:
        raise ValueError("selected input bytes do not match the frozen input")

    rows = list(csv.reader(io.StringIO(contents.decode("utf-8"), newline=""), strict=True))
    if len(rows) != 1025:
        raise ValueError("fixed CPU input requires a header and exactly 1024 samples")
    if rows[0] != [f"x{i}" for i in range(16)] + ["label"]:
        raise ValueError("fixed CPU input requires exactly x0 through x15 and label")
    features, labels = [], []
    for row in rows[1:]:
        if len(row) != 17 or row[-1] not in ("0", "1"):
            raise ValueError("each sample requires 16 features and a binary integer label")
        values = [float(value) for value in row[:-1]]
        if any(not math.isfinite(value) or abs(value) > 3.4028234663852886e38 for value in values):
            raise ValueError("features must be finite float32 values")
        features.append(values)
        labels.append(int(row[-1]))
    return contents, digest, features, labels


def write_json(path, value):
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    os.replace(temporary, path)


def recipe_deadline(_signal, _frame):
    raise TimeoutError("CPU03_RECIPE_DEADLINE: 30-second training budget expired")


def terminate(signum, _frame):
    # The container's PID 1 must explicitly handle SIGTERM. SystemExit unwinds
    # the open metrics file and reports nonzero termination to the owning runtime.
    raise SystemExit(128 + signum)


def main():
    signal.signal(signal.SIGTERM, terminate)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--expected-input-sha256", required=True)
    parser.add_argument("--expected-input-bytes", required=True, type=int)
    parser.add_argument("--learning-rate", default="0.01")
    parser.add_argument(
        "--recipe", choices=("success", "fail", "slow-stop"), default="success",
        help="registered managed recipe; never an ordinary user training parameter",
    )
    args = parser.parse_args()

    canonical_rate, optimizer_rate = learning_rate(args.learning_rate)
    if os.environ.get("WORLD_SIZE", "1") != "1":
        raise ValueError("the fixed CPU recipe requires WORLD_SIZE=1")
    data = checked_path(args.data)
    output = checked_path(args.output)
    check_output(output)
    input_bytes, input_sha256, feature_values, label_values = load_selected_input(
        data, args.expected_input_sha256, args.expected_input_bytes,
    )
    if args.recipe == "slow-stop":
        signal.signal(signal.SIGALRM, recipe_deadline)
        signal.alarm(30)

    # Validate the entire selected input before importing the training runtime,
    # constructing tensors, or creating any output directory.
    import torch
    from torch import nn

    features = torch.tensor(feature_values, dtype=torch.float32, device="cpu")
    labels = torch.tensor(label_values, dtype=torch.long, device="cpu")
    output.mkdir(parents=True, exist_ok=True)
    checked_path(output)
    check_output(output)

    torch.set_num_threads(1)
    torch.manual_seed(42)
    model = nn.Sequential(nn.Linear(16, 32), nn.ReLU(), nn.Linear(32, 2))
    optimizer = torch.optim.Adam(model.parameters(), lr=optimizer_rate)
    criterion = nn.CrossEntropyLoss()
    with torch.no_grad():
        initial_loss = float(criterion(model(features), labels))

    if args.recipe == "slow-stop":
        print(json.dumps({
            "schema": "ani.cpu03.recipe.v1",
            "recipe": "slow-stop",
            "max_steps": 48,
            "step_delay_seconds": 0.2,
            "deadline_seconds": 30,
        }), flush=True)

    step = 0
    with (output / "metrics.jsonl").open("x", encoding="utf-8", buffering=1) as metrics:
        for _ in range(3):
            for indices in torch.randperm(len(features)).split(64):
                optimizer.zero_grad(set_to_none=True)
                loss = criterion(model(features[indices]), labels[indices])
                if not torch.isfinite(loss):
                    raise ValueError("non-finite training loss")
                loss.backward()
                optimizer.step()
                step += 1
                entry = {
                    "schema": "ani.metric.v1",
                    "step": step,
                    "name": "train.loss",
                    "value": float(loss.detach()),
                    "rank": 0,
                    "timestamp": datetime.now(timezone.utc).isoformat(),
                }
                line = json.dumps(entry)
                metrics.write(line + "\n")
                print(line, flush=True)
                if args.recipe == "fail" and step == 5:
                    raise RuntimeError("CPU03_RECIPE_FAILURE: failed after five real optimizer steps")
                if args.recipe == "slow-stop":
                    time.sleep(0.2)

    model.eval()
    with torch.no_grad():
        final_loss = float(criterion(model(features), labels))
    if not math.isfinite(final_loss) or final_loss >= initial_loss:
        raise ValueError("fixed baseline training did not reduce full-dataset loss")

    checkpoint_temporary = output / "model.pt.tmp"
    torch.save(model.state_dict(), checkpoint_temporary)
    os.replace(checkpoint_temporary, output / "model.pt")
    write_json(output / "model_config.json", {
        "architecture": "mlp-16-32-2",
        "input_dim": 16,
        "hidden_dim": 32,
        "output_dim": 2,
        "dtype": "float32",
    })
    write_json(output / "summary.json", {
        "samples": len(feature_values),
        "epochs": 3,
        "batch_size": 64,
        "learning_rate": canonical_rate,
        "steps": step,
        "world_size": 1,
        "device": "cpu",
        "torch_version": torch.__version__,
        "input_sha256": input_sha256,
        "input_bytes": str(len(input_bytes)),
        "initial_loss": initial_loss,
        "final_loss": final_loss,
    })
    files = []
    for name in ("model.pt", "model_config.json", "metrics.jsonl", "summary.json"):
        contents = (output / name).read_bytes()
        files.append({
            "path": name,
            "bytes": str(len(contents)),
            "sha256": hashlib.sha256(contents).hexdigest(),
        })
    write_json(output / "result.json", {
        "schema": "ani.output-candidate.v1",
        "input_sha256": input_sha256,
        "deployable": False,
        "files": files,
    })
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as error:
        print(f"CPU training failed: {type(error).__name__}: {error}", file=sys.stderr)
        sys.exit(1)

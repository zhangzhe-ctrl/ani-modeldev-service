"""Train the fixed CPU-P01 MLP from a selected CSV.

The result is a workload candidate. This command has no control-plane clients
and does not mark an Execution successful or publish files to object storage.
"""

import argparse
import csv
from datetime import datetime, timezone
import hashlib
import io
import json
import math
import os
from pathlib import Path
import sys

import torch
from torch import nn


def write_json(path, value):
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    os.replace(temporary, path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--expected-input-sha256", required=True)
    parser.add_argument("--expected-input-bytes", required=True, type=int)
    args = parser.parse_args()

    # This first behavior slice consumes valid input. Rejection of a mismatched
    # expected digest/size is the next RED/GREEN cycle, before this is deployable.
    input_bytes = Path(args.data).read_bytes()
    input_sha256 = hashlib.sha256(input_bytes).hexdigest()
    rows = list(csv.DictReader(io.StringIO(input_bytes.decode("utf-8"), newline="")))
    features = torch.tensor(
        [[float(row[f"x{i}"]) for i in range(16)] for row in rows],
        dtype=torch.float32,
        device="cpu",
    )
    labels = torch.tensor([int(row["label"]) for row in rows], dtype=torch.long, device="cpu")
    output = Path(args.output)
    output.mkdir(parents=True)

    torch.set_num_threads(1)
    torch.manual_seed(42)
    model = nn.Sequential(nn.Linear(16, 32), nn.ReLU(), nn.Linear(32, 2))
    optimizer = torch.optim.Adam(model.parameters(), lr=0.01)
    criterion = nn.CrossEntropyLoss()
    with torch.no_grad():
        initial_loss = float(criterion(model(features), labels))

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
        "samples": len(rows),
        "epochs": 3,
        "batch_size": 64,
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

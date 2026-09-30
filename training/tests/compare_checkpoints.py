"""Compare trusted CPU03 checkpoint tensors, not serialization metadata."""

import json
from pathlib import Path
import sys

import torch


def main():
    checkpoints = [
        torch.load(Path(value) / "model.pt", map_location="cpu", weights_only=True)
        for value in sys.argv[1:]
    ]
    if len(checkpoints) != 2:
        raise ValueError("two checkpoints are required")
    first, second = checkpoints
    if first.keys() != second.keys() or len(first) != 4:
        raise ValueError("checkpoint parameter keys differ")
    for key in first:
        if first[key].shape != second[key].shape:
            raise ValueError("checkpoint parameter shapes differ")
        if not torch.isfinite(first[key]).all() or not torch.isfinite(second[key]).all():
            raise ValueError("checkpoint parameters are not finite")
    if all(torch.equal(first[key], second[key]) for key in first):
        raise ValueError("different effective learning rates produced identical parameter tensors")
    print(json.dumps({"different_parameters": True}))


if __name__ == "__main__":
    main()

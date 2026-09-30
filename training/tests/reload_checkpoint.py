"""Independent trusted-checkpoint consumer for the CPU03 module test only.

This process imports no trainer code. It does not claim S3, BFF or L4 evidence.
"""

import json
from pathlib import Path
import sys

import torch
from torch import nn


def main():
    root = Path(sys.argv[1])
    config = json.loads((root / "model_config.json").read_text(encoding="utf-8"))
    expected = {
        "architecture": "mlp-16-32-2",
        "input_dim": 16,
        "hidden_dim": 32,
        "output_dim": 2,
        "dtype": "float32",
    }
    if config != expected:
        raise ValueError("checkpoint configuration does not match the fixed CPU recipe")
    weights = torch.load(root / "model.pt", map_location="cpu", weights_only=True)
    if not isinstance(weights, dict) or len(weights) != 4:
        raise ValueError("expected the four tensors of the fixed MLP state_dict")
    if sum(tensor.numel() for tensor in weights.values()) != 610:
        raise ValueError("wrong MLP parameter count")
    for tensor in weights.values():
        if not torch.is_tensor(tensor) or not torch.isfinite(tensor).all():
            raise ValueError("checkpoint contains non-finite or non-tensor weights")
    if not any(torch.count_nonzero(tensor).item() for tensor in weights.values()):
        raise ValueError("checkpoint weights are empty")
    model = nn.Sequential(nn.Linear(16, 32), nn.ReLU(), nn.Linear(32, 2))
    model.load_state_dict(weights, strict=True)
    model.eval()
    with torch.inference_mode():
        result = model(torch.zeros((4, 16), dtype=torch.float32))
    if result.shape != (4, 2) or not torch.isfinite(result).all():
        raise ValueError("checkpoint forward result is invalid")
    print(json.dumps({"shape": list(result.shape), "device": str(result.device)}))


if __name__ == "__main__":
    main()

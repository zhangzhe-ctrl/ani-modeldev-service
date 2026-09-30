"""Generate the CPU-P01 fixed CSV on Fedora or in an authorized cluster Job.

Adapted from the frozen CPU-P01 reference/v04/cpu-baseline/make_data.py.
This is fixture preparation, never an operation performed by the trainer.
"""

import argparse
import csv
from pathlib import Path
import random


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    rng = random.Random(20260929)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("x", newline="", encoding="utf-8") as stream:
        writer = csv.writer(stream)
        writer.writerow([f"x{i}" for i in range(16)] + ["label"])
        for _ in range(1024):
            features = [round(rng.uniform(-1, 1), 6) for _ in range(16)]
            label = int(features[0] + 0.5 * features[1] - 0.25 * features[2] > 0)
            writer.writerow(features + [label])


if __name__ == "__main__":
    main()

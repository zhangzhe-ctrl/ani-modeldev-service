"""CPU-P01 workload command seam; implementation awaits a Fedora RED run."""

import argparse
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--expected-input-sha256", required=True)
    parser.add_argument("--expected-input-bytes", required=True, type=int)
    parser.parse_args()
    print("CPU03_NOT_IMPLEMENTED: fixed CSV training is not implemented", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())

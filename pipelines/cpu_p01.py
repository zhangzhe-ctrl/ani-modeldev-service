"""The single CPU-P01 KFP assembly entry; compile on Fedora only."""
import argparse
import json


def compile_pipeline(configuration, output):
    raise NotImplementedError("CPU_PIPELINE_ASSEMBLY_NOT_IMPLEMENTED")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True)
    parser.add_argument("--output", required=True)
    options = parser.parse_args()
    with open(options.config, encoding="utf-8") as source:
        compile_pipeline(json.load(source), options.output)

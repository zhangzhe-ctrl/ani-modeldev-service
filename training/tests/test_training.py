"""CPU03 behavior tests: run only on Fedora or in an authorized cluster Job."""

import csv
import hashlib
import json
import math
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


TRAINING_ROOT = Path(__file__).resolve().parents[1]


class FixedCSVTrainingTest(unittest.TestCase):
    def run_python(self, *arguments, timeout=120):
        environment = os.environ.copy()
        environment.update({"CUDA_VISIBLE_DEVICES": "", "WORLD_SIZE": "1"})
        return subprocess.run(
            [sys.executable, "-I", *map(str, arguments)],
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
            env=environment,
        )

    def test_fixed_csv_produces_48_step_checkpoint_that_a_new_process_can_reload(self):
        dependency = self.run_python(
            "-c",
            "import torch; print(torch.__version__); "
            "assert torch.version.cuda is None, 'CPU wheel required'",
        )
        self.assertEqual(
            dependency.returncode,
            0,
            "ENVIRONMENT_NOT_READY (not a behavior RED): a working CPU PyTorch "
            f"environment is required\n{dependency.stdout}\n{dependency.stderr}",
        )
        with tempfile.TemporaryDirectory(prefix="cpu03-training-") as temporary:
            root = Path(temporary)
            data = root / "selected-input.csv"
            output = root / "execution-output"
            fixture = self.run_python(TRAINING_ROOT / "make_data.py", "--output", data)
            self.assertEqual(fixture.returncode, 0, fixture.stderr)
            with data.open(newline="", encoding="utf-8") as stream:
                records = list(csv.reader(stream))
            self.assertEqual(records[0], [f"x{i}" for i in range(16)] + ["label"])
            self.assertEqual(len(records) - 1, 1024)
            input_bytes = data.read_bytes()
            input_sha256 = hashlib.sha256(input_bytes).hexdigest()

            trained = self.run_python(
                TRAINING_ROOT / "train_mlp.py",
                "--data", data,
                "--output", output,
                "--expected-input-sha256", input_sha256,
                "--expected-input-bytes", len(input_bytes),
            )
            self.assertEqual(
                trained.returncode,
                0,
                "valid fixed CSV must train and produce a checkpoint; "
                f"exit={trained.returncode}\n{trained.stdout}\n{trained.stderr}",
            )
            self.assertEqual(data.read_bytes(), input_bytes, "selected input must remain unchanged")
            expected_files = {"model.pt", "model_config.json", "metrics.jsonl", "summary.json"}
            self.assertEqual({path.name for path in output.iterdir()}, expected_files | {"result.json"})
            summary = json.loads((output / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["samples"], 1024)
            self.assertEqual(summary["epochs"], 3)
            self.assertEqual(summary["batch_size"], 64)
            self.assertEqual(summary["steps"], 48)
            self.assertEqual(summary["world_size"], 1)
            self.assertEqual(summary["device"], "cpu")
            self.assertEqual(summary["input_sha256"], input_sha256)
            self.assertEqual(summary["input_bytes"], str(len(input_bytes)))
            self.assertTrue(math.isfinite(summary["initial_loss"]))
            self.assertTrue(math.isfinite(summary["final_loss"]))
            self.assertLess(summary["final_loss"], summary["initial_loss"])

            metrics = [
                json.loads(line)
                for line in (output / "metrics.jsonl").read_text(encoding="utf-8").splitlines()
            ]
            self.assertEqual([entry["step"] for entry in metrics], list(range(1, 49)))
            for entry in metrics:
                self.assertEqual(entry["name"], "train.loss")
                self.assertEqual(entry["rank"], 0)
                self.assertTrue(math.isfinite(entry["value"]))
                self.assertGreaterEqual(entry["value"], 0)

            candidate = json.loads((output / "result.json").read_text(encoding="utf-8"))
            self.assertEqual(candidate["schema"], "ani.output-candidate.v1")
            self.assertEqual(candidate["input_sha256"], input_sha256)
            self.assertFalse(candidate["deployable"])
            self.assertEqual(len(candidate["files"]), 4)
            self.assertEqual({entry["path"] for entry in candidate["files"]}, expected_files)
            for entry in candidate["files"]:
                actual = (output / entry["path"]).read_bytes()
                self.assertGreater(len(actual), 0)
                self.assertEqual(entry["bytes"], str(len(actual)))
                self.assertEqual(entry["sha256"], hashlib.sha256(actual).hexdigest())

            reloaded = self.run_python(Path(__file__).with_name("reload_checkpoint.py"), output)
            self.assertEqual(reloaded.returncode, 0, reloaded.stderr)
            self.assertEqual(json.loads(reloaded.stdout), {"shape": [4, 2], "device": "cpu"})


if __name__ == "__main__":
    unittest.main()

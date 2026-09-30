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
    def run_python(self, *arguments, timeout=120, environment_overrides=None):
        environment = os.environ.copy()
        environment.update({"CUDA_VISIBLE_DEVICES": "", "WORLD_SIZE": "1"})
        environment.update(environment_overrides or {})
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

    def test_invalid_input_is_rejected_before_training_and_preserves_paths(self):
        dependency = self.run_python(
            "-c", "import torch; assert torch.version.cuda is None, 'CPU wheel required'"
        )
        self.assertEqual(
            dependency.returncode, 0,
            f"ENVIRONMENT_NOT_READY (not a behavior RED)\n{dependency.stderr}",
        )
        cases = (
            "wrong_sha256", "wrong_size", "negative_size",
            "missing_row", "extra_row", "missing_feature", "extra_feature",
            "duplicate_feature", "invalid_label", "fractional_label",
            "nan_feature", "inf_feature", "negative_inf_feature",
            "world_size_two", "world_size_zero", "world_size_invalid",
            "symlink_data", "symlink_output", "nonempty_output",
        )
        with tempfile.TemporaryDirectory(prefix="cpu03-rejected-input-") as temporary:
            root = Path(temporary)
            fixture_path = root / "fixture.csv"
            generated = self.run_python(TRAINING_ROOT / "make_data.py", "--output", fixture_path)
            self.assertEqual(generated.returncode, 0, generated.stderr)
            with fixture_path.open(newline="", encoding="utf-8") as stream:
                fixed_rows = list(csv.reader(stream))
            for case in cases:
                with self.subTest(case=case):
                    case_root = root / case
                    case_root.mkdir()
                    data = case_root / "selected-input.csv"
                    output = case_root / "execution-output"
                    rows = [row.copy() for row in fixed_rows]
                    if case == "missing_row":
                        rows.pop()
                    elif case == "extra_row":
                        rows.append(rows[-1].copy())
                    elif case == "missing_feature":
                        for row in rows:
                            row.pop(15)
                    elif case == "extra_feature":
                        rows[0].insert(16, "x16")
                        for row in rows[1:]:
                            row.insert(16, "0.0")
                    elif case == "duplicate_feature":
                        rows[0][15] = "x14"
                    elif case == "invalid_label":
                        rows[1][-1] = "2"
                    elif case == "fractional_label":
                        rows[1][-1] = "0.5"
                    elif case in ("nan_feature", "inf_feature", "negative_inf_feature"):
                        rows[1][0] = {
                            "nan_feature": "NaN",
                            "inf_feature": "Inf",
                            "negative_inf_feature": "-Inf",
                        }[case]
                    with data.open("x", newline="", encoding="utf-8") as stream:
                        csv.writer(stream).writerows(rows)
                    selected_bytes = data.read_bytes()
                    expected_sha256 = hashlib.sha256(selected_bytes).hexdigest()
                    expected_size = len(selected_bytes)
                    if case == "wrong_sha256":
                        expected_sha256 = "0" * 64
                    elif case == "wrong_size":
                        expected_size += 1
                    elif case == "negative_size":
                        expected_size = -1
                    if case == "symlink_data":
                        target = case_root / "protected-input.csv"
                        data.rename(target)
                        data.symlink_to(target)

                    protected_output = None
                    if case == "symlink_output":
                        protected_output = case_root / "protected-output"
                        protected_output.mkdir()
                        output.symlink_to(protected_output, target_is_directory=True)
                    elif case == "nonempty_output":
                        output.mkdir()
                        protected_output = output
                        (output / "sentinel.txt").write_bytes(b"existing execution bytes\n")
                    world_size = {
                        "world_size_two": "2",
                        "world_size_zero": "0",
                        "world_size_invalid": "not-an-integer",
                    }.get(case, "1")
                    rejected = self.run_python(
                        TRAINING_ROOT / "train_mlp.py",
                        "--data", data,
                        "--output", output,
                        "--expected-input-sha256", expected_sha256,
                        "--expected-input-bytes", expected_size,
                        environment_overrides={"WORLD_SIZE": world_size},
                    )
                    self.assertNotEqual(
                        rejected.returncode, 0,
                        f"{case} must be rejected before computation\n"
                        f"{rejected.stdout}\n{rejected.stderr}",
                    )
                    events = []
                    for line in rejected.stdout.splitlines():
                        try:
                            events.append(json.loads(line))
                        except json.JSONDecodeError:
                            continue
                    self.assertFalse(
                        any(isinstance(event, dict) and event.get("name") == "train.loss" for event in events),
                        "rejection must not emit a completed optimization step",
                    )
                    self.assertEqual(data.read_bytes(), selected_bytes, "input bytes must be preserved")
                    self.assertEqual(data.is_symlink(), case == "symlink_data")
                    if protected_output is None:
                        self.assertFalse(output.exists(), "rejection must not create an output directory")
                        self.assertFalse(output.is_symlink())
                    else:
                        self.assertEqual(
                            {path.name for path in protected_output.iterdir()},
                            {"sentinel.txt"} if case == "nonempty_output" else set(),
                            "existing output must not acquire training artifacts",
                        )
                        if case == "nonempty_output":
                            self.assertEqual(
                                (protected_output / "sentinel.txt").read_bytes(), b"existing execution bytes\n",
                            )
                        self.assertEqual(output.is_symlink(), case == "symlink_output")


if __name__ == "__main__":
    unittest.main()

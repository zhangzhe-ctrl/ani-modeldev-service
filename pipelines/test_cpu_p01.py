"""Verify the compiled public IR contract, not a hand-built DAG substitute."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from google.protobuf import json_format
from kfp.pipeline_spec import pipeline_spec_pb2
import yaml


class PipelineAssembly(unittest.TestCase):
    def test_compiled_pipeline_preserves_execution_chain_and_failure_close(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            configuration = {
                "component_image": "registry.example.test/modeldev-step@sha256:" + "a" * 64,
                "owner_config_map": "cpu-step-owner-fixture",
                "storage_credentials_secret": "cpu-step-temporary-storage-fixture",
                "storage_class": "cpu-workspace-fixture",
                "workspace_size": "2Gi",
                "workspace_access_mode": "ReadWriteOnce",
            }
            config = root / "compile.json"
            config.write_text(json.dumps(configuration), encoding="utf-8")
            output = root / "pipeline.yaml"
            result = subprocess.run(
                [sys.executable, str(Path(__file__).with_name("cpu_p01.py")),
                 "--config", str(config), "--output", str(output)],
                capture_output=True, text=True, check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            documents = list(yaml.safe_load_all(output.read_text(encoding="utf-8")))
            ir, platform = documents
            json_format.ParseDict(ir, pipeline_spec_pb2.PipelineSpec())
            inputs = ir["root"]["inputDefinitions"]["parameters"]
            self.assertEqual(set(inputs), {"execution_id", "spec_hash"})
            self.assertTrue(all(p["parameterType"] == "STRING" and "defaultValue" not in p for p in inputs.values()))
            tasks = ir["root"]["dag"]["tasks"]
            self.assertEqual(set(tasks), {"createpvc", "prepare", "train-wait", "collect", "publish", "close"})
            for before, after in [("createpvc", "prepare"), ("prepare", "train-wait"), ("train-wait", "collect"), ("collect", "publish"), ("publish", "close")]:
                self.assertIn(before, tasks[after]["dependentTasks"])
            self.assertEqual(tasks["close"]["triggerPolicy"]["strategy"], "ALL_UPSTREAM_TASKS_COMPLETED")
            close_component = ir["components"][tasks["close"]["componentRef"]["name"]]
            self.assertEqual(close_component["inputDefinitions"]["parameters"]["candidate"]["defaultValue"], "")
            for name, task in tasks.items():
                self.assertFalse(task.get("cachingOptions", {}).get("enableCache", False))
                self.assertEqual(int(task.get("retryPolicy", {}).get("maxRetryCount", 0)), 0)
                if name == "createpvc":
                    continue  # Official KFP resource primitive, not a runnable image.
                executor = ir["components"][task["componentRef"]["name"]]["executorLabel"]
                container = ir["deploymentSpec"]["executors"][executor]["container"]
                self.assertEqual(container["image"], configuration["component_image"])
                self.assertEqual(container["command"], ["/ani-modeldev-step", name])
                self.assertIn("{{$.pipeline_job_uuid}}", container["args"])
                self.assertIn("{{$.pipeline_task_uuid}}", container["args"])
                self.assertNotIn("sh", container["command"])
                k8s = platform["platforms"]["kubernetes"]["deploymentSpec"]["executors"][executor]
                self.assertEqual(bool(k8s.get("pvcMount")), name in {"prepare", "collect", "publish"})
                self.assertEqual(bool(k8s.get("secretAsVolume")), name in {"prepare", "publish"})
                self.assertEqual({item["name"] for item in k8s["fieldPathAsEnv"]}, {"ANI_POD_NAME", "ANI_POD_UID", "ANI_POD_NAMESPACE"})
            self.assertNotIn("deletepvc", output.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()

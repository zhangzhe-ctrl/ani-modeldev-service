"""Real command logic; only Kubernetes/KFP/S3/SSH are external peers."""

import copy
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import cleanup as c
import ssh_peer


class Peer:
    def __init__(self):
        self.kube_objects, self.kfp_objects, self.artifacts = {}, {}, {}
        self.controllers, self.extra_runs, self.recurring = [], [], []
        self.deleted, self.identities = [], 0
        self.fail_delete = None

    def identity(self):
        self.identities += 1
        return dict(c.IDENTITY)

    def kube(self, method, path, body=None):
        if method == "GET":
            return copy.deepcopy(self.kube_objects.get(path))
        self.deleted.append(("kube", path, body))
        obj = self.kube_objects[path]
        assert body["preconditions"] == {"uid": obj["metadata"]["uid"], "resourceVersion": obj["metadata"]["resourceVersion"]}
        assert body["propagationPolicy"] == "Orphan" and "gracePeriodSeconds" not in body
        if self.fail_delete == "kube":
            raise c.Refusal("EXTERNAL_DELETE_RESPONSE_LOST")
        del self.kube_objects[path]
        return {}

    def kube_list(self, path):
        return copy.deepcopy(self.controllers) if path.startswith("/apis/apps/v1/namespaces/" + c.NS + "/deployments") else []

    def kfp(self, method, path):
        if method == "GET":
            return copy.deepcopy(self.kfp_objects.get(path))
        self.deleted.append(("kfp", path, None))
        del self.kfp_objects[path]
        return {}

    def kfp_list(self, kind, filters=None):
        if kind == "jobs":
            return self.recurring
        if kind == "runs":
            return [o["run"] for p, o in self.kfp_objects.items() if "/runs/" in p] + self.extra_runs
        return [o for p, o in self.kfp_objects.items() if "/pipeline_versions/" in p]

    def s3(self, operation, target):
        key = (target["bucket"], target["key"], target["version_id"])
        if operation == "READ_VERSION":
            return copy.deepcopy(self.artifacts.get(key))
        self.deleted.append(("s3", key, None))
        del self.artifacts[key]
        return {}


class CommandTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        path = Path(self.directory.name) / "root-verified-source.json"
        path.write_text('{"external_fixture":"original close, release ACK and stopped sources; exact exclusive objects"}')
        self.proof = {"path": str(path), "sha256": c.sha(path.read_bytes())}
        self.execution = {"execution_id": "11111111-1111-4111-8111-111111111111", "operation_id": "22222222-2222-4222-8222-222222222222", "spec_hash": "a" * 64,
                          "environment": {"namespace_name": c.NS, "namespace_uid": c.NS_UID, "binding_id": c.BINDING, "experiment_id": "experiment-original"},
                          "implementation": {"pipeline_id": "pipeline-original", "pipeline_version_id": "version-original"},
                          "runtime": {"Workspace": {"PVCUID": "33333333-3333-4333-8333-333333333333", "PVCName": "original-workspace"}}}
        self.inventory = {"executions": [self.execution], "runs": [{"run_id": "run-original"}], "authorities": [], "inputs": [{"input_version_id": "input-original"}]}
        self.peer = Peer()
        self.scope = {"schema": "ani.modeldev.external-cleanup.scope.v1", "inventory_sha256": c.INVENTORY_SHA, "binding_id": c.BINDING,
                      "connection_sha256": "c" * 64,
                      "kube_targets": [], "kfp_targets": [], "kfp_preserved": [], "s3_versions": [], "artifact_inventory_proof": self.proof, "configuration_inventory_proof": self.proof}
        for kind, name, uid in (("PersistentVolumeClaim", "original-workspace", self.execution["runtime"]["Workspace"]["PVCUID"]),
                                ("ConfigMap", "old-tenant-config", "44444444-4444-4444-8444-444444444444")):
            obj = {"apiVersion": "v1", "kind": kind, "metadata": {"namespace": c.NS, "name": name, "uid": uid, "resourceVersion": "17"}}
            target = {"api_version": "v1", "kind": kind, "namespace": c.NS, "name": name, "uid": uid, "resource_version": "17", "object_sha256": c.sha(obj), "exclusive_owner_proof": self.proof}
            self.peer.kube_objects[c.object_path(target)] = obj
            self.scope["kube_targets"].append(target)
        for kind, identifier in (("runs", "run-original"), ("pipeline_versions", "version-original"), ("pipelines", "pipeline-original"), ("experiments", "experiment-original")):
            obj = {"pipeline_id" if kind == "pipelines" else "id": identifier}
            if kind == "runs":
                obj = {"run": {"id": identifier, "resource_references": [{"key": {"type": "EXPERIMENT", "id": "experiment-original"}}]}}
            target = {"kind": kind, "id": identifier, "object_sha256": c.sha(obj), "exclusive_owner_proof": self.proof}
            self.peer.kfp_objects[c.kfp_path(target)] = obj
            self.scope["kfp_targets"].append(target)
        artifact = {"bucket": "original-kfp-bucket", "key": "artifacts/md/run-original/step/executor-logs", "version_id": "immutable-original-version", "etag": '"abc"', "size_bytes": 8, "sha256": "b" * 64, "run_id": "run-original", "exclusive_owner_proof": self.proof}
        self.scope["s3_versions"].append(artifact)
        self.peer.artifacts[(artifact["bucket"], artifact["key"], artifact["version_id"])] = {k: artifact[k] for k in ("version_id", "etag", "size_bytes", "sha256")}
        self.scope_sha = c.sha(self.scope)
        now = dt.datetime.now(dt.timezone.utc)
        self.gate = {"schema": "ani.modeldev.external-cleanup.prerequisites.v1", "inventory_sha256": c.INVENTORY_SHA, "scope_sha256": self.scope_sha,
                     "identity": c.IDENTITY, "observed_at": (now - dt.timedelta(seconds=1)).isoformat(), "expires_at": (now + dt.timedelta(minutes=4)).isoformat(),
                     "executions": [{k: self.execution[k] for k in ("execution_id", "operation_id", "spec_hash")} | {"qualification_proof": self.proof}],
                     "stopped_sources": [{"component": name, "source_uid": "original-source-uid", "source_revision": "original-revision", "proof": self.proof} for name in ("MODELDEV_WORKERS", "GOVERNANCE_CREATE_PUBLISHER", "LEGACY_REPLAY_AND_PUBLICATION")],
                     "maintenance_503": {"method": "POST", "path": "/api/v1/modeldev/executions", "status": 503, "request_id": "authenticated-original-probe", "proof": self.proof}, "all_original_charges_and_ack_proof": self.proof}
        self.journal = []

    def command(self, scope=True, gate=True):
        return c.Cleanup(self.inventory, self.scope if scope else None, self.scope_sha, self.gate if gate else None, c.sha(self.gate), self.peer, "c" * 64)

    def save(self, receipt):
        self.journal.append(copy.deepcopy(receipt))

    def test_vertical_plan_then_exact_apply_preserves_namespace_shared_secret_inputs(self):
        command = self.command()
        plan = command.plan()
        self.assertEqual(plan["blockers"], [])
        self.assertEqual(self.peer.deleted, [])
        receipt = command.apply(plan, self.save)
        self.assertEqual(receipt["state"], "APPLIED_EXTERNAL_ONLY")
        self.assertEqual(len(receipt["actions"]), 7)
        kinds = [kind for kind, _, _ in self.peer.deleted]
        self.assertEqual(kinds, ["kube", "kube", "s3", "kfp", "kfp", "kfp", "kfp"])
        self.assertEqual(plan["input_metadata"]["action"], "DELEGATE_TO_FIXED_DB_CLEANUP_AFTER_EXTERNAL_COMPLETION")
        self.assertGreaterEqual(self.peer.identities, 9)
        for entry in self.journal[:-1]:
            self.assertEqual(entry["state"], "PARTIAL")

    def test_missing_scope_or_fresh_closed_proof_never_adopts_404(self):
        self.peer.kube_objects.clear()
        for scope, gate, expected in ((False, True, "EXACT_UID"), (True, False, "FRESH_ORIGINAL_CLOSED")):
            plan = self.command(scope, gate).plan()
            self.assertTrue(plan["blockers"][0].startswith(expected))
            self.assertEqual(plan["original_identity_candidates"]["source"], "FROZEN_INVENTORY_ONLY_NOT_CURRENT_OBSERVATION")
            self.assertEqual(plan["original_identity_candidates"]["pvcs"][0]["uid"], self.execution["runtime"]["Workspace"]["PVCUID"])
            with self.assertRaises(c.Refusal):
                self.command(scope, gate).apply(plan, self.save)
        self.assertEqual(self.peer.deleted, [])

    def test_changed_uid_rv_or_bytes_blocks_before_any_delete(self):
        command = self.command()
        plan = command.plan()
        obj = next(iter(self.peer.kube_objects.values()))
        obj["metadata"]["uid"] = "55555555-5555-4555-8555-555555555555"
        with self.assertRaisesRegex(c.Refusal, "DRIFTED"):
            command.apply(plan, self.save)
        self.assertEqual(self.peer.deleted, [])

    def test_foreign_controller_pvc_reference_blocks_even_suspended(self):
        self.peer.controllers = [{"metadata": {"uid": "foreign-controller", "namespace": c.NS}, "spec": {"replicas": 0, "template": {"spec": {"volumes": [{"persistentVolumeClaim": {"claimName": "original-workspace"}}]}}}}]
        self.assertIn("LIVE_CONTROLLER_OR_FOREIGN_REFERENCE", self.command().plan()["blockers"][0])
        self.assertEqual(self.peer.deleted, [])

    def test_original_workflow_presence_blocks_run_delete_even_with_old_close_proof(self):
        original = {"api_version": "argoproj.io/v1alpha1", "kind": "Workflow", "namespace": c.NS, "name": "original-workflow", "uid": "66666666-6666-4666-8666-666666666666"}
        self.inventory["authorities"] = [{"workflow_uid": original["uid"], "workflow_name": original["name"]}]
        self.peer.kube_objects[c.object_path(original)] = {"metadata": {"uid": original["uid"]}, "status": {"phase": "Succeeded"}}
        self.assertIn("ORIGINAL_WORKLOAD_NOT_ABSENT", self.command().plan()["blockers"][0])
        self.assertEqual(self.peer.deleted, [])

    def test_foreign_run_or_recurring_job_reference_retains_catalog(self):
        for kind in ("run", "recurring"):
            with self.subTest(kind=kind):
                self.peer.extra_runs, self.peer.recurring = [], []
                foreign = {"id": "other-owner", "pipeline_spec": {"pipeline_id": "pipeline-original"}}
                (self.peer.extra_runs if kind == "run" else self.peer.recurring).append(foreign)
                self.assertIn("KFP_REFERENCED_BY_OTHER", self.command().plan()["blockers"][0])
        self.assertEqual(self.peer.deleted, [])

    def test_artifact_version_or_bytes_conflict_blocks_all_deletes(self):
        next(iter(self.peer.artifacts.values()))["sha256"] = "c" * 64
        self.assertEqual(self.command().plan()["blockers"], ["ARTIFACT_VERSION_OR_BYTES_DIFFERS"])
        self.assertEqual(self.peer.deleted, [])

    def test_lost_delete_response_has_durable_unknown_and_no_retry(self):
        command = self.command()
        plan = command.plan()
        self.peer.fail_delete = "kube"
        with self.assertRaisesRegex(c.Refusal, "RESPONSE_LOST"):
            command.apply(plan, self.save)
        self.assertEqual(len(self.peer.deleted), 1)
        self.assertEqual(self.journal[-1]["state"], "PARTIAL")
        self.assertEqual(self.journal[-1]["actions"][-1]["state"], "RESULT_UNKNOWN")

    def test_proof_file_drift_or_expiry_refuses(self):
        self.gate["expires_at"] = (dt.datetime.now(dt.timezone.utc) - dt.timedelta(seconds=1)).isoformat()
        self.assertEqual(self.command().plan()["blockers"], ["PREREQUISITE_NOT_FRESH"])
        self.gate["expires_at"] = (dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=4)).isoformat()
        Path(self.proof["path"]).write_text("changed")
        self.assertEqual(self.command().plan()["blockers"], ["EVIDENCE_BYTES_SHA256_DIFFERS"])

    def test_production_inventory_pin_cannot_be_replaced_by_fixture(self):
        with self.assertRaisesRegex(c.Refusal, "ORIGINAL_INVENTORY_DIFFERS"):
            c.fixed_inventory(c.canonical(self.inventory))


class TransportTests(unittest.TestCase):
    def test_real_node_subprocess_preserves_cas_and_exact_s3_version(self):
        # Real node-helper process and real temporary files; only the external
        # kubectl/aws executables stand in for their remote services.
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            kubectl = root / "kubectl"
            kubectl.write_text("#!" + sys.executable + "\n" + '''import base64,json,sys
args=sys.argv[1:]
if 'current-context' in args:print('kubernetes-admin@ani-kcn-full')
elif 'view' in args:print(json.dumps({'clusters':[{'cluster':{'server':'https://lb.kubesphere.local:6443'}}]}))
elif 'secret' in args:print(json.dumps({'data':{'config.yaml':base64.b64encode(b'cluster_id: f39bf6e2-f3ed-4191-ada3-caeebf86c220').decode()}}))
elif 'kube-system' in args:print(json.dumps({'metadata':{'uid':'3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24'}}))
elif 'ani-modeldev-tenant-a-v21' in args:print(json.dumps({'metadata':{'uid':'94f3afc0-7548-4099-9e11-3fa7fdf6cc67'}}))
elif 'delete' in args:
 body=json.load(sys.stdin)
 assert body['preconditions']=={'uid':'original-uid','resourceVersion':'17'}
 assert body['propagationPolicy']=='Orphan' and 'gracePeriodSeconds' not in body
 print('{}')
else:
 print(json.dumps({'metadata':{'uid':'original-uid','resourceVersion':'17'}}))
''')
            kubectl.chmod(0o700)
            aws = root / "aws"
            aws.write_text("#!" + sys.executable + "\n" + '''import json,sys
args=sys.argv[1:]
assert args[args.index('--version-id')+1]=='immutable-original-version'
assert '--no-verify-ssl' not in args
if 'head-object' in args:print(json.dumps({'VersionId':'immutable-original-version','ETag':'"abc"','ContentLength':8}))
elif 'get-object' in args:
 open(args[-1],'wb').write(b'original')
 print('{}')
elif 'delete-object' in args:print('{}')
else:raise SystemExit(2)
''')
            aws.chmod(0o700)
            private = root / "credentials"
            private.write_text("external boundary fixture")
            private.chmod(0o600)
            connection = {"endpoint": "https://original", "ca_file": str(private), "credentials_file": str(private), "config_file": str(private), "profile": "original"}
            target = {"bucket": "original", "key": "artifacts/md/original/log", "version_id": "immutable-original-version"}
            requests = [{"op": "identity"}, {"op": "kube", "method": "DELETE", "path": "/api/v1/namespaces/original/persistentvolumeclaims/original", "body": {"preconditions": {"uid": "original-uid", "resourceVersion": "17"}, "propagationPolicy": "Orphan"}}, {"op": "s3", "operation": "READ_VERSION", "target": target, "connection": connection}, {"op": "s3", "operation": "DELETE_VERSION", "target": target, "connection": connection}]
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"])
            process = subprocess.run([sys.executable, "-u", "-c", ssh_peer.NODE], input="".join(json.dumps(r) + "\n" for r in requests), text=True, capture_output=True, env=env, timeout=10)
            self.assertEqual(process.returncode, 0, process.stderr)
            result = [json.loads(line) for line in process.stdout.splitlines()]
            self.assertEqual(result[0], {"ok": c.IDENTITY})
            self.assertEqual(result[1], {"ok": {}})
            self.assertEqual(result[2], {"ok": {"version_id": "immutable-original-version", "etag": '"abc"', "size_bytes": 8, "sha256": c.sha(b"original")}})
            self.assertEqual(result[3], {"ok": {}})

    def test_pagination_reads_all_pages_and_rejects_loop(self):
        peer = ssh_peer.SSHPeer({"kfp": {"endpoint": "https://original", "ca_file": "/ca", "token_file": "/token", "control_pod_uid": "original"}, "s3": {"endpoint": "https://original", "ca_file": "/ca", "credentials_file": "/private", "config_file": "/config", "profile": "original"}})
        with mock.patch.object(peer, "kfp", side_effect=[{"runs": [{"id": "one"}], "next_page_token": "next"}, {"runs": [{"id": "two"}]}]) as method:
            self.assertEqual([r["id"] for r in peer.kfp_list("runs")], ["one", "two"])
            self.assertIn("page_token=next", method.call_args_list[1].args[1])
        with mock.patch.object(peer, "kfp", return_value={"versions": [{"id": "original-version"}], "total_size": 1}):
            self.assertEqual(peer.kfp_list("pipeline_versions"), [{"id": "original-version"}])
        with mock.patch.object(peer, "kfp", return_value={"pipeline_versions": [{"id": "unrecognized-shape"}]}):
            with self.assertRaisesRegex(c.Refusal, "UNVERIFIED"):
                peer.kfp_list("pipeline_versions")
        with mock.patch.object(peer, "kube", side_effect=[{"items": [], "metadata": {"continue": "same"}}, {"items": [], "metadata": {"continue": "same"}}]):
            with self.assertRaisesRegex(c.Refusal, "PAGINATION_LIMIT"):
                peer.kube_list("/api/v1/namespaces/original/pods")

    def test_node_program_syntax_and_transport_fixed_host_no_retry(self):
        compile(ssh_peer.NODE, "node-helper", "exec")
        self.assertIn("AWS_MAX_ATTEMPTS='1'", ssh_peer.NODE)
        self.assertNotIn("--force", ssh_peer.NODE)
        self.assertNotIn("--no-verify-ssl", ssh_peer.NODE)


if __name__ == "__main__":
    unittest.main()

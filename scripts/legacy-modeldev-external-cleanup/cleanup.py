#!/usr/bin/env python3
"""One-time exact-object disposal; no workload close or namespace deletion."""

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import urllib.parse

INVENTORY_SHA = "aa5420bd1c2ad02f373509d625a99080b4c6721050de1bcdb3496ad1d15a44d2"
ENTRY = "kubercloud@100.64.128.12"
IDENTITY = {"ssh": ENTRY, "context": "kubernetes-admin@ani-kcn-full",
            "api_server": "https://lb.kubesphere.local:6443",
            "cluster_id": "f39bf6e2-f3ed-4191-ada3-caeebf86c220",
            "kube_system_uid": "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24"}
NS = "ani-modeldev-tenant-a-v21"
NS_UID = "94f3afc0-7548-4099-9e11-3fa7fdf6cc67"
BINDING = "e91440de-0cef-4d88-b9d8-83126f8f8132"
# Read-only guards from the existing frozen biz.LookupLegacyRetirementScope;
# these add absence checks, never a close/log exception or new deletion target.
RETIREMENT_WORKFLOWS = {
    "1cce950b-d6e9-453e-aa11-fffd8846224b": ("bbd4ad22-5b1a-448a-b71a-1069e18adc2f", "ft-sw2pr", "edcf4a1d-cdf9-44da-9f28-20593d79469f"),
    "4a1a92f7-4d94-439d-a8d7-1833f38c7dbc": ("2c372d09-70a8-4557-8b60-20f7ecd7d861", "ft-j4ncq", "1e9521b7-fc75-41f2-a406-690a7c142b73"),
    "eca4d47b-5911-4f0f-8b45-89c1be775925": ("5f04cda7-4c66-4f16-a552-053ebd4e5b6f", "md-kpwkw", "d2acb3df-3979-4d60-aed5-16db0ce440fb")}
RETIREMENT_PODS = {"49c82c2b-f819-4a03-b7b2-1789999cdd2e", "d8fb153a-5763-4ae7-8d44-63e3f996a830"}
KUBE = {("v1", "PersistentVolumeClaim"): "persistentvolumeclaims",
        ("v1", "ConfigMap"): "configmaps", ("v1", "ServiceAccount"): "serviceaccounts",
        ("rbac.authorization.k8s.io/v1", "RoleBinding"): "rolebindings",
        ("rbac.authorization.k8s.io/v1", "Role"): "roles"}
WORKLOAD = {("v1", "Pod"): "pods", ("batch/v1", "Job"): "jobs",
            ("argoproj.io/v1alpha1", "Workflow"): "workflows",
            ("trainer.kubeflow.org/v1alpha1", "TrainJob"): "trainjobs",
            ("jobset.x-k8s.io/v1alpha2", "JobSet"): "jobsets",
            ("scheduling.volcano.sh/v1beta1", "PodGroup"): "podgroups",
            ("trainer.kubeflow.org/v1alpha1", "TrainingRuntime"): "trainingruntimes"}
NAME = re.compile(r"[a-z0-9]([a-z0-9.-]*[a-z0-9])?\Z")
UUID = re.compile(r"[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}\Z")


class Refusal(Exception):
    pass


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def sha(value):
    return hashlib.sha256(value if isinstance(value, bytes) else canonical(value)).hexdigest()


def read_file(path, expected=None, limit=16 << 20):
    p = Path(path)
    if not p.is_absolute() or p.is_symlink() or not p.is_file() or p.stat().st_size > limit:
        raise Refusal("EVIDENCE_FILE_INVALID")
    raw = p.read_bytes()
    if expected is not None and (not re.fullmatch(r"[a-f0-9]{64}", expected) or sha(raw) != expected):
        raise Refusal("EVIDENCE_BYTES_SHA256_DIFFERS")
    return raw


def decode(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise Refusal("DUPLICATE_JSON_FIELD")
            result[key] = value
        return result
    return json.loads(raw, object_pairs_hook=pairs)


def proof(value):
    if set(value) != {"path", "sha256"}:
        raise Refusal("SOURCE_PROOF_INVALID")
    read_file(value["path"], value["sha256"])


def instant(value):
    return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))


def fixed_inventory(raw):
    if sha(raw) != INVENTORY_SHA:
        raise Refusal("ORIGINAL_INVENTORY_DIFFERS")
    data = decode(raw)
    if len(data["executions"]) != 50 or len(data["runs"]) != 50 or len(data["inputs"]) != 10:
        raise Refusal("ORIGINAL_SCOPE_DIFFERS")
    return data


def object_path(target):
    version, kind = target["api_version"], target["kind"]
    resource = (KUBE | WORKLOAD).get((version, kind))
    if not resource or target["namespace"] not in (NS, "ani-system") or not NAME.fullmatch(target["name"]):
        raise Refusal("KUBERNETES_TARGET_INVALID")
    base = "/api/v1" if version == "v1" else "/apis/" + version
    return base + "/namespaces/" + target["namespace"] + "/" + resource + "/" + target["name"]


def kfp_path(target):
    # V1 GET pipeline requires a latest version and fails after the last version
    # is removed. V2 represents that same pipeline independently; no cascade.
    api = "v2beta1" if target["kind"] == "pipelines" else "v1beta1"
    return "/apis/" + api + "/" + target["kind"] + "/" + target["id"]


def walk(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


def inventory_scope(data):
    executions = {e["execution_id"]: e for e in data["executions"]}
    runs = {r["run_id"] for r in data["runs"]}
    versions = {e["implementation"]["pipeline_version_id"]: e["implementation"]["pipeline_id"] for e in executions.values()}
    experiments = {e["environment"]["experiment_id"] for e in executions.values()}
    original, pvcs = {}, {}
    for execution in executions.values():
        env = execution["environment"]
        if env["namespace_name"] != NS or env["namespace_uid"] != NS_UID or env["binding_id"] != BINDING:
            raise Refusal("ORIGINAL_ENVIRONMENT_DIFFERS")
        runtime = execution.get("runtime") or {}
        workspace = runtime.get("Workspace")
        if workspace:
            pvcs[workspace["PVCUID"]] = workspace["PVCName"]
        for resource in walk(runtime):
            if all(resource.get(k) for k in ("APIVersion", "Kind", "Namespace", "Name", "UID")):
                target = {"api_version": resource["APIVersion"], "kind": resource["Kind"],
                          "namespace": resource["Namespace"], "name": resource["Name"], "uid": resource["UID"]}
                if (target["api_version"], target["kind"]) in WORKLOAD:
                    original[target["uid"]] = target
        binding = runtime.get("RuntimeTemplate", {}).get("Binding", {}) if runtime.get("RuntimeTemplate") else {}
        if binding.get("UID"):
            original[binding["UID"]] = {"api_version": "trainer.kubeflow.org/v1alpha1", "kind": "TrainingRuntime",
                                        "namespace": NS, "name": binding["Name"], "uid": binding["UID"]}
    for authority in data["authorities"]:
        original[authority["workflow_uid"]] = {"api_version": "argoproj.io/v1alpha1", "kind": "Workflow",
                                             "namespace": NS, "name": authority["workflow_name"], "uid": authority["workflow_uid"]}
    for execution, (run, name, uid) in RETIREMENT_WORKFLOWS.items():
        if execution in executions:
            if run not in runs:
                raise Refusal("ORIGINAL_RETIREMENT_RUN_DIFFERS")
            original[uid] = {"api_version": "argoproj.io/v1alpha1", "kind": "Workflow", "namespace": NS, "name": name, "uid": uid}
    return executions, runs, versions, experiments, original, pvcs


class Cleanup:
    def __init__(self, inventory, scope, scope_sha, gate, gate_sha, peer, connection_sha):
        self.inventory, self.scope, self.scope_sha = inventory, scope, scope_sha
        self.gate, self.gate_sha, self.peer = gate, gate_sha, peer
        self.connection_sha = connection_sha
        self.executions, self.runs, self.versions, self.experiments, self.workloads, self.pvcs = inventory_scope(inventory)

    def qualify(self):
        scope, gate = self.scope, self.gate
        if scope is None:
            raise Refusal("EXACT_UID_CONFIGURATION_AND_ARTIFACT_SCOPE_UNKNOWN")
        if set(scope) != {"schema", "inventory_sha256", "binding_id", "connection_sha256", "kube_targets", "kfp_targets", "kfp_preserved", "s3_versions", "artifact_inventory_proof", "configuration_inventory_proof"} or scope["schema"] != "ani.modeldev.external-cleanup.scope.v1" or scope["inventory_sha256"] != INVENTORY_SHA or scope["binding_id"] != BINDING or scope["connection_sha256"] != self.connection_sha:
            raise Refusal("EXTERNAL_SCOPE_INVALID")
        proof(scope["artifact_inventory_proof"])
        proof(scope["configuration_inventory_proof"])
        if gate is None:
            raise Refusal("FRESH_ORIGINAL_CLOSED_AND_STOPPED_SOURCE_PROOF_UNKNOWN")
        if set(gate) != {"schema", "inventory_sha256", "scope_sha256", "identity", "observed_at", "expires_at", "executions", "stopped_sources", "maintenance_503", "all_original_charges_and_ack_proof"} or gate["schema"] != "ani.modeldev.external-cleanup.prerequisites.v1" or gate["inventory_sha256"] != INVENTORY_SHA or gate["scope_sha256"] != self.scope_sha or gate["identity"] != IDENTITY:
            raise Refusal("PREREQUISITE_SCOPE_DIFFERS")
        now = dt.datetime.now(dt.timezone.utc)
        observed, expires = instant(gate["observed_at"]), instant(gate["expires_at"])
        if observed > now or now >= expires or expires - observed > dt.timedelta(minutes=5):
            raise Refusal("PREREQUISITE_NOT_FRESH")
        if {r["execution_id"] for r in gate["executions"]} != set(self.executions) or len(gate["executions"]) != len(self.executions):
            raise Refusal("ORIGINAL_CLOSE_SCOPE_INCOMPLETE")
        for row in gate["executions"]:
            original = self.executions[row["execution_id"]]
            if set(row) != {"execution_id", "operation_id", "spec_hash", "qualification_proof"} or any(row[k] != original[k] for k in ("operation_id", "spec_hash")):
                raise Refusal("ORIGINAL_CLOSE_IDENTITY_DIFFERS")
            proof(row["qualification_proof"])
        if {r["component"] for r in gate["stopped_sources"]} != {"MODELDEV_WORKERS", "GOVERNANCE_CREATE_PUBLISHER", "LEGACY_REPLAY_AND_PUBLICATION"} or len(gate["stopped_sources"]) != 3:
            raise Refusal("STOPPED_SOURCES_INCOMPLETE")
        for row in gate["stopped_sources"]:
            if not row.get("source_uid") or not row.get("source_revision") or set(row) != {"component", "source_uid", "source_revision", "proof"}:
                raise Refusal("STOPPED_SOURCE_IDENTITY_UNKNOWN")
            proof(row["proof"])
        probe = gate["maintenance_503"]
        if set(probe) != {"method", "path", "status", "request_id", "proof"} or probe["method"] != "POST" or probe["path"] != "/api/v1/modeldev/executions" or probe["status"] != 503 or not probe["request_id"]:
            raise Refusal("REAL_AUTHENTICATED_MAINTENANCE_503_UNKNOWN")
        proof(probe["proof"])
        proof(gate["all_original_charges_and_ack_proof"])
        for group in ("kube_targets", "kfp_targets", "s3_versions"):
            if not isinstance(scope[group], list) or len(scope[group]) > 512:
                raise Refusal("EXTERNAL_TARGET_LIMIT")
        if {r["uid"] for r in scope["kube_targets"] if r["kind"] == "PersistentVolumeClaim"} != set(self.pvcs):
            raise Refusal("ORIGINAL_PVC_SCOPE_INCOMPLETE")
        candidates = ({("runs", r) for r in self.runs} | {("experiments", r) for r in self.experiments} |
                      {("pipeline_versions", r) for r in self.versions} | {("pipelines", r) for r in self.versions.values()})
        classified = [(r["kind"], r["id"]) for r in scope["kfp_targets"]]
        for row in scope["kfp_preserved"]:
            if set(row) != {"kind", "id", "reason", "shared_owner_proof"} or row["kind"] not in ("pipelines", "pipeline_versions") or row["reason"] != "SHARED_NON_MODELDEV_OWNER":
                raise Refusal("KFP_PRESERVATION_INVALID")
            proof(row["shared_owner_proof"])
            classified.append((row["kind"], row["id"]))
        if len(classified) != len(set(classified)) or set(classified) != candidates:
            raise Refusal("KFP_OWNERSHIP_CLASSIFICATION_INCOMPLETE")
        versions_deleted = {r["id"] for r in scope["kfp_targets"] if r["kind"] == "pipeline_versions"}
        for row in scope["kfp_targets"]:
            if row["kind"] == "pipelines" and any(v not in versions_deleted for v, parent in self.versions.items() if parent == row["id"]):
                raise Refusal("PIPELINE_HAS_PRESERVED_OR_UNREVIEWED_VERSION")
        s3_keys = [(r["bucket"], r["key"], r["version_id"]) for r in scope["s3_versions"]]
        if len(s3_keys) != len(set(s3_keys)):
            raise Refusal("DUPLICATE_S3_VERSION")

    def kube_target(self, target):
        if set(target) != {"api_version", "kind", "namespace", "name", "uid", "resource_version", "object_sha256", "exclusive_owner_proof"} or (target["api_version"], target["kind"]) not in KUBE or not UUID.fullmatch(target["uid"]) or not target["resource_version"]:
            raise Refusal("EXACT_KUBE_TARGET_INVALID")
        object_path(target)
        if target["kind"] == "PersistentVolumeClaim" and (target["namespace"] != NS or self.pvcs.get(target["uid"]) != target["name"]):
            raise Refusal("PVC_NOT_ORIGINAL")
        if target["namespace"] == "ani-system" and target["kind"] != "ConfigMap":
            raise Refusal("SHARED_SYSTEM_IDENTITY_RETAINED")
        proof(target["exclusive_owner_proof"])
        obj = self.peer.kube("GET", object_path(target))
        if obj is None:
            return {"state": "ABSENT", "original_uid": target["uid"]}
        meta = obj["metadata"]
        if meta.get("uid") != target["uid"] or meta.get("resourceVersion") != target["resource_version"] or sha(obj) != target["object_sha256"]:
            raise Refusal("ORIGINAL_KUBE_UID_RV_OR_BYTES_DIFFERS")
        if obj.get("apiVersion") != target["api_version"] or obj.get("kind") != target["kind"] or meta.get("namespace") != target["namespace"] or meta.get("name") != target["name"] or meta.get("deletionTimestamp"):
            raise Refusal("KUBE_OBJECT_IDENTITY_OR_DELETION_DIFFERS")
        return {"state": "PRESENT", "original_uid": target["uid"], "resource_version": meta["resourceVersion"], "object_sha256": sha(obj)}

    def absence(self):
        observations = []
        for target in sorted(self.workloads.values(), key=lambda r: r["uid"]):
            obj = self.peer.kube("GET", object_path(target))
            if obj is not None:
                raise Refusal("ORIGINAL_WORKLOAD_NOT_ABSENT:" + target["uid"])
            observations.append({"uid": target["uid"], "state": "ABSENT"})
        return observations

    def references(self):
        # Complete paginated lists, including templates of suspended controllers.
        objects = []
        for namespace in sorted({NS, "ani-system"}):
            for api, resource in (("/api/v1", "pods"), ("/apis/apps/v1", "deployments"), ("/apis/apps/v1", "replicasets"), ("/apis/apps/v1", "statefulsets"), ("/apis/apps/v1", "daemonsets"), ("/apis/batch/v1", "jobs"), ("/apis/batch/v1", "cronjobs")):
                objects.extend(self.peer.kube_list(api + "/namespaces/" + namespace + "/" + resource))
        for version, resource in (("argoproj.io/v1alpha1", "workflows"), ("trainer.kubeflow.org/v1alpha1", "trainjobs"), ("trainer.kubeflow.org/v1alpha1", "trainingruntimes"), ("jobset.x-k8s.io/v1alpha2", "jobsets"), ("scheduling.volcano.sh/v1beta1", "podgroups")):
            objects.extend(self.peer.kube_list("/apis/" + version + "/namespaces/" + NS + "/" + resource))
        # Subjects may reference an old SA across namespaces, including cluster
        # bindings. Do not limit this reference check to the old namespace.
        for resource in ("rolebindings", "clusterrolebindings"):
            objects.extend(self.peer.kube_list("/apis/rbac.authorization.k8s.io/v1/" + resource))
        targets = self.scope["kube_targets"]
        deleting = {r["uid"] for r in targets}
        for obj in objects:
            metadata = obj["metadata"]
            if metadata.get("uid") in deleting:
                continue
            identifiers = set(metadata.get("labels", {}).values()) | set(metadata.get("annotations", {}).values())
            if metadata.get("uid") in set(self.workloads) | RETIREMENT_PODS or any(owner.get("uid") in self.workloads for owner in metadata.get("ownerReferences", [])) or identifiers.intersection(set(self.executions) | self.runs):
                raise Refusal("ORIGINAL_OR_LATE_WORKLOAD_STILL_PRESENT")
            namespace = obj["metadata"].get("namespace")
            for target in targets:
                kind, name = target["kind"], target["name"]
                if namespace != target["namespace"] and kind != "ServiceAccount":
                    continue
                for node in walk(obj):
                    referenced = (kind == "PersistentVolumeClaim" and node.get("persistentVolumeClaim", {}).get("claimName") == name or
                                  kind == "ConfigMap" and any(node.get(key, {}).get("name") == name for key in ("configMap", "configMapRef", "configMapKeyRef")) or
                                  kind == "ServiceAccount" and namespace == target["namespace"] and (node.get("serviceAccountName", "default") == name or node.get("serviceAccount") == name) or
                                  kind == "ServiceAccount" and node.get("kind") == "ServiceAccount" and node.get("name") == name and node.get("namespace", namespace) == target["namespace"] or
                                  kind == "Role" and namespace == target["namespace"] and node.get("roleRef", {}).get("kind") == "Role" and node["roleRef"].get("name") == name)
                    if referenced:
                        raise Refusal("LIVE_CONTROLLER_OR_FOREIGN_REFERENCE:" + target["uid"])
        return sha(objects)

    def kfp_target(self, target):
        if set(target) != {"kind", "id", "object_sha256", "exclusive_owner_proof"}:
            raise Refusal("KFP_TARGET_INVALID")
        valid = {"runs": self.runs, "experiments": self.experiments, "pipeline_versions": set(self.versions), "pipelines": set(self.versions.values())}
        if target["kind"] not in valid or target["id"] not in valid[target["kind"]]:
            raise Refusal("KFP_ID_NOT_ORIGINAL")
        proof(target["exclusive_owner_proof"])
        obj = self.peer.kfp("GET", kfp_path(target))
        if obj is None:
            return {"state": "ABSENT"}
        body = obj.get("run", obj) if target["kind"] == "runs" else obj
        id_field = "pipeline_id" if target["kind"] == "pipelines" else "id"
        if body.get(id_field) != target["id"] or sha(obj) != target["object_sha256"]:
            raise Refusal("KFP_ID_OR_REVIEWED_BYTES_DIFFERS")
        return {"state": "PRESENT", "object_sha256": sha(obj)}

    def kfp_references(self):
        targets = self.scope["kfp_targets"]
        allowed = {kind: {t["id"] for t in targets if t["kind"] == kind} for kind in ("runs", "experiments", "pipeline_versions", "pipelines")}
        result = {}
        for kind in ("runs", "jobs"):
            rows = self.peer.kfp_list(kind)
            result[kind] = rows
            for row in rows:
                if kind == "runs" and row.get("id") in allowed["runs"]:
                    continue
                referenced = canonical(row).decode()
                if any(identifier in referenced for group in ("experiments", "pipeline_versions", "pipelines") for identifier in allowed[group]):
                    raise Refusal("KFP_REFERENCED_BY_OTHER_RUN_OR_RECURRING_JOB")
        for pipeline in sorted(allowed["pipelines"]):
            rows = self.peer.kfp_list("pipeline_versions", {"resource_key.type": "PIPELINE", "resource_key.id": pipeline})
            result[pipeline] = rows
            if any(r.get("id") not in allowed["pipeline_versions"] for r in rows):
                raise Refusal("PIPELINE_HAS_UNREVIEWED_VERSION")
        return sha(result)

    def s3_target(self, target):
        if set(target) != {"bucket", "key", "version_id", "etag", "size_bytes", "sha256", "run_id", "exclusive_owner_proof"} or target["run_id"] not in self.runs or not target["key"].startswith("artifacts/md/" + target["run_id"] + "/") or target["version_id"] in ("", "null") or not NAME.fullmatch(target["bucket"]) or not re.fullmatch(r"[a-f0-9]{64}", target["sha256"]) or not 0 <= target["size_bytes"] <= 64 << 20:
            raise Refusal("EXACT_EXCLUSIVE_ARTIFACT_VERSION_INVALID")
        proof(target["exclusive_owner_proof"])
        observed = self.peer.s3("READ_VERSION", target)
        if observed is None:
            return {"state": "ABSENT"}
        expected = {k: target[k] for k in ("version_id", "etag", "size_bytes", "sha256")}
        if observed != expected:
            raise Refusal("ARTIFACT_VERSION_OR_BYTES_DIFFERS")
        return {"state": "PRESENT", **observed}

    def plan(self):
        plan = {"schema": "ani.modeldev.external-cleanup.plan.v1", "inventory_sha256": INVENTORY_SHA,
                "scope_sha256": self.scope_sha, "prerequisites_sha256": self.gate_sha, "identity": None,
                "connection_sha256": self.connection_sha,
                "kube": [], "kfp": [], "s3": [], "blockers": [], "manifest_sha256": "",
                "original_identity_candidates": {
                    "source": "FROZEN_INVENTORY_ONLY_NOT_CURRENT_OBSERVATION",
                    "pvcs": [{"name": name, "uid": uid, "namespace": NS} for uid, name in sorted(self.pvcs.items())],
                    "workloads": sorted(self.workloads.values(), key=lambda r: r["uid"]),
                    "runs": sorted(self.runs), "pipeline_versions": dict(sorted(self.versions.items())),
                    "experiments": sorted(self.experiments)},
                "input_metadata": {"action": "DELEGATE_TO_FIXED_DB_CLEANUP_AFTER_EXTERNAL_COMPLETION", "ids": sorted(r["input_version_id"] for r in self.inventory["inputs"])},
                "retained": ["Namespace", "standard A/B Namespaces", "shared image Secrets", "external inputs/base models", "independent READY Models", "old input/output physical bytes"]}
        try:
            self.qualify()
            plan["identity"] = self.peer.identity()
            if plan["identity"] != IDENTITY:
                raise Refusal("FULL_CLUSTER_IDENTITY_DIFFERS")
            plan["original_workload_absence"] = self.absence()
            plan["controller_references_sha256"] = self.references()
            if len({object_path(t) for t in self.scope["kube_targets"]}) != len(self.scope["kube_targets"]):
                raise Refusal("DUPLICATE_KUBE_TARGET")
            for target in self.scope["kube_targets"]:
                plan["kube"].append({"target": target, "observation": self.kube_target(target)})
            plan["kfp_references_sha256"] = self.kfp_references()
            for target in self.scope["kfp_targets"]:
                plan["kfp"].append({"target": target, "observation": self.kfp_target(target)})
            for target in self.scope["s3_versions"]:
                plan["s3"].append({"target": target, "observation": self.s3_target(target)})
            self.qualify()
        except Refusal as exc:
            plan["blockers"].append(str(exc))
        plan["manifest_sha256"] = sha(plan)
        return plan

    def apply(self, reviewed, persist):
        expected = dict(reviewed)
        expected["manifest_sha256"] = ""
        if reviewed.get("manifest_sha256") != sha(expected) or reviewed.get("blockers"):
            raise Refusal("REVIEWED_PLAN_NOT_QUALIFIED")
        if self.plan() != reviewed:
            raise Refusal("REVIEWED_PLAN_DRIFTED")
        receipt = {"schema": "ani.modeldev.external-cleanup.receipt.v1", "plan": reviewed,
                   "started_at": dt.datetime.now(dt.timezone.utc).isoformat(), "state": "PARTIAL", "actions": []}
        persist(receipt)
        # RB before Role/SA; KFP Run records only after original workloads absent.
        rank = {"RoleBinding": 0, "Role": 1, "ServiceAccount": 2, "ConfigMap": 3, "PersistentVolumeClaim": 4}
        actions = [("kube", r) for r in sorted(reviewed["kube"], key=lambda r: rank[r["target"]["kind"]])]
        actions += [("s3", r) for r in reviewed["s3"]]
        kfp_rank = {"runs": 0, "pipeline_versions": 1, "pipelines": 2, "experiments": 3}
        actions += [("kfp", r) for r in sorted(reviewed["kfp"], key=lambda r: kfp_rank[r["target"]["kind"]])]
        for kind, row in actions:
            target = row["target"]
            if row["observation"]["state"] == "ABSENT":
                receipt["actions"].append({"kind": kind, "target": target, "state": "ALREADY_ABSENT_WITH_SEPARATE_ORIGINAL_CLOSE_PROOF"})
                persist(receipt)
                continue
            self.qualify()
            if self.peer.identity() != IDENTITY:
                raise Refusal("FULL_CLUSTER_IDENTITY_DIFFERS")
            self.references()
            observe = {"kube": self.kube_target, "kfp": self.kfp_target, "s3": self.s3_target}[kind]
            if observe(target) != row["observation"]:
                raise Refusal("TARGET_CHANGED_BEFORE_DELETE")
            pending = {"kind": kind, "target": target, "state": "RESULT_UNKNOWN"}
            receipt["actions"].append(pending)
            persist(receipt)  # Durable intent before external request; never auto-retry.
            if kind == "kube":
                self.peer.kube("DELETE", object_path(target), {"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": {"uid": target["uid"], "resourceVersion": target["resource_version"]}, "propagationPolicy": "Orphan"})
            elif kind == "kfp":
                self.kfp_references()
                if target["kind"] == "pipelines" and self.peer.kfp_list("pipeline_versions", {"resource_key.type": "PIPELINE", "resource_key.id": target["id"]}):
                    raise Refusal("PIPELINE_VERSIONS_NOT_CONFIRMED_ABSENT")
                self.peer.kfp("DELETE", kfp_path(target))
            else:
                self.peer.s3("DELETE_VERSION", target)
            if observe(target)["state"] != "ABSENT":
                pending["state"] = "REQUESTED_NOT_CONFIRMED"
                persist(receipt)
                raise Refusal("DELETE_NOT_CONFIRMED_NO_FORCE_OR_RETRY")
            pending["state"] = "CONFIRMED_ABSENT"
            persist(receipt)
        self.qualify()
        self.absence()
        receipt["state"] = "APPLIED_EXTERNAL_ONLY"
        receipt["completed_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        persist(receipt)
        return receipt


def main():
    from ssh_peer import SSHPeer
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inventory", required=True)
    parser.add_argument("--scope")
    parser.add_argument("--scope-sha256")
    parser.add_argument("--prerequisites")
    parser.add_argument("--prerequisites-sha256")
    parser.add_argument("--connection", required=True, help="root-reviewed nonsecret KFP/S3 node-local connection references")
    parser.add_argument("--connection-sha256", required=True)
    parser.add_argument("--mode", choices=("plan", "apply"), default="plan")
    parser.add_argument("--plan")
    parser.add_argument("--plan-sha256", help="reviewed plan file byte SHA256")
    parser.add_argument("--receipt", help="new mode0600 durable journal path, apply only")
    args = parser.parse_args()
    inventory = fixed_inventory(read_file(args.inventory))
    scope = decode(read_file(args.scope, args.scope_sha256)) if args.scope else None
    gate = decode(read_file(args.prerequisites, args.prerequisites_sha256)) if args.prerequisites else None
    if args.scope and not args.scope_sha256 or args.prerequisites and not args.prerequisites_sha256:
        raise Refusal("ROOT_APPROVED_BYTE_HASH_REQUIRED")
    connection = decode(read_file(args.connection, args.connection_sha256, limit=64 << 10))
    command = Cleanup(inventory, scope, args.scope_sha256, gate, args.prerequisites_sha256, SSHPeer(connection), args.connection_sha256)
    if args.mode == "plan":
        print(json.dumps(command.plan(), indent=2))
        return
    if not args.plan or not args.plan_sha256 or not args.receipt:
        raise Refusal("APPLY_REQUIRES_REVIEWED_PLAN_AND_NEW_RECEIPT")
    reviewed = decode(read_file(args.plan, args.plan_sha256))
    path = Path(args.receipt)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(fd)
    def persist(receipt):
        temporary = path.with_name(path.name + ".next")
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(json.dumps(receipt, indent=2) + "\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    result = command.apply(reviewed, persist)
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (Refusal, ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError) as error:
        print("EXTERNAL_CLEANUP_BLOCKED_OR_PARTIAL: " + (str(error) if isinstance(error, Refusal) else type(error).__name__), file=sys.stderr)
        sys.exit(1)

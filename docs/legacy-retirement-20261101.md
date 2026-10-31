# Bounded retirement of the three original ModelDev executions

This command is built only from the old service base `21bf4bd6540d462a61c91b629f26250899583223` on branch `codex/modeldev-legacy-retirement`. It is not wired into either service runtime. The direct human approval has SHA256 `53c1b292af08051c06f254bf05308734e1db9b87e4c48da9ecd3e47a3e11b60e`. The three execution IDs, original namespace UID, Run/Workflow UIDs and official raw v1 archive hashes are compiled into `biz.LookupLegacyRetirementScope`.

The business sequence is original admission/frozen dispatch and unique Run → original fence/generation/revision → current complete owner/training writer and PVC checks → original CLOSED transaction → durable step-exit receipt → exact original Pod finalizer CAS and UID/RV cleanup → original ordered compute cleanup, maximum three attempts → the actual original Governance release receipt and account invariant. All still-unmet qualifications refuse progress. Ordinary runtime entry points retain their original live-Workflow and captured-log requirements and reject maintenance facts.

Two CPU training streams remain explicitly `TRAINING_LOGS_UNRECOVERABLE`, with original Pod UID/containerID and the actual previous kubelet-error bytes/hash. No `TrainingLog`, `LogsReady`, captured flag or log lines are created. The official v1 `pipeline_runtime.workflow_manifest` is read in memory, checked against its fixed original bytes SHA, and kept separate from current Kubernetes API evidence. Current Workflow GETs must really report NotFound. The approved `image_contract` no-process exception is limited to the original third execution and retains the complete retry/parent/child/template/process-field checks.

The third execution already has Governance generic `resource-release-v1` receipt `23600748-d80d-5ceb-be35-664a995bb7db`, event `54c1fb78-da48-4586-990e-333ed8090aef`, create operation `1bc185ad-0977-4b3f-bb1c-404bc37e94fa`, resource `eca4d47b-5911-4f0f-8b45-89c1be775925`, reason `RESOURCE_RELEASED`, payload SHA256 `3e63b8ddc427e1228169804c6a6d28c34899318224257119b0c788b522ca7cad`. The old MD anchor and outbox are absent. Calling the ordinary MD `SealGPURelease` would generate a different event and `ABORTED_CLEANED` reason; maintenance explicitly rejects it. After real close and cleanup, the command replays the exact original four fields through the existing ModelDev mTLS Governance client, requires actual `ALREADY_APPLIED`, and rechecks the exact original charge plus both GPU account equalities before CAS-recording an independent maintenance journal. It never synthesizes an MD release scope/outbox or applies another refund. This is the bounded maintenance interpretation of spec 13.3.7 and ADR 0002; their release authority is preserved.

## Execution and evidence

Heavy generation, tests and builds run on Fedora only. `--mode=plan` opens the MD database with `default_transaction_read_only=on`, reuses the repository's RR read-only complete qualification, refuses non-read Kubernetes requests, and reads the original control workload's already-projected tokens. It creates no tokens. Config contains only node-local protected file references; DSNs, token/private-key bytes and full Workflow manifests never enter stdout or evidence. The SSH entry identity is checked in full using the existing kubercloud kubectl configuration; business Kube/KFP calls retain the original control identity.

```sh
sudo env KUBECONFIG=/home/kubercloud/.kube/config \
  /tmp/modeldev-legacy-retirement-command-stripped \
  --config /tmp/modeldev-legacy-retirement-20261101/config.json --mode plan
```

The JSON plan states each original identity and review reason, its current exit graph and immutable proof digest, and every refusal. A plan is not a closure or cleanup receipt. Apply accepts only the exact reviewed plan file SHA and a fresh root-generated stop-admission receipt bound to that plan and the actual source probe receipt hash. The complete three-entry qualification is repeated before the first write, followed by another full cluster identity check and fresh stop-admission gate. Each repository write still uses the original aggregate lock/CAS. A restart reads the explicit journal and continues the original bounded receipts, without generating a new close generation or attempt budget.

The required stop-admission JSON has exactly these fields; the actual source probe must prove HTTP 503 and unchanged acceptance/charge/lease facts. `root_apply_authorized` stays false until root reviews the real plan and explicitly authorizes apply. The tool checks five-minute gate freshness and thirty-minute plan freshness.

```json
{
  "schema": "ani.modeldev.legacy-retirement.stopped-admission.v1",
  "observed_at": "<fresh actual UTC RFC3339>",
  "source_evidence_sha256": "<SHA256 of actual root probe receipt>",
  "reviewed_plan_sha256": "<SHA256 of exact reviewed plan file>",
  "cluster_id": "f39bf6e2-f3ed-4191-ada3-caeebf86c220",
  "kube_system_uid": "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24",
  "governance_deployment_uid": "4ae87828-5f2c-4b27-a8b8-d348aeb86ee5",
  "governance_image": "<actual pinned deployed image>",
  "create_paused": true,
  "create_http_status": 503,
  "create_reason": "MODELDEV_CREATE_MAINTENANCE",
  "active_create_leases": 0,
  "root_apply_authorized": false
}
```

Apply additionally takes `--plan`, `--plan-sha256`, `--stopped-admission-receipt` and `--stopped-admission-sha256`. No apply has been run. The first node transfer failed with SSH exit 255, so live plan and all mutation/business acceptance remain NOT_RUN until the specified SSH entrance is available. Component, real-PostgreSQL, source-gate and live evidence are kept separate under `.scratch/legacy-retirement/evidence/`.

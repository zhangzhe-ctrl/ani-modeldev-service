# ModelDev native protection

`render.py` emits the fixed `modeldev-bootstrap-v1`,
`modeldev-environment-config-v1`, and `modeldev-workload-guard-v1` policies,
their `-binding` bindings, and the finite platform bootstrap roles. It performs
no cluster operation and accepts no tenant list or Namespace UID label.

Input has exactly `principals`, `node_principals`, `kfp_ca_key`, and
`network_metadata_writer`, and `network_status_writer`.
The principals map contains the verified control, webhook, kfp_api, argo,
trainer, jobset, job_controller, persistence_agent, and maintenance usernames.
Node usernames are exact `system:node:NAME` identities; the renderer adds no
prefix or group exemption. `kfp_ca_key` is the actual CABUNDLE_KEY_NAME.
The separate verified network ServiceAccount may only update an existing Pod's
`networking.kubercloud.com/vnic` annotation through the main Pod resource. Its
Pod spec, labels, owner references, finalizers, ModelDev identity markers and all
other annotations must remain unchanged. This identity is not a workload creator
or a general controller update exception, and receives no new RBAC permissions.
The separate verified Multus status ServiceAccount may only UPDATE the existing
Pod status subresource to set `k8s.v1.cni.cncf.io/network-status`, preserving the
current status as well as spec/identity metadata. This follows its pinned native
UpdateStatus call. Neither network identity may alter the other's annotation or
any other Pod metadata/spec through these branches.

Install and verify the native policies before granting bootstrap CREATE/bind.
Do not replace an old running environment's policies before its ordered close,
resource cleanup, and original GPU release ACK have completed. Namespace
ownership is read through the two network labels; live UID is verified by the
ModelDev owner and webhook, because native namespaceObject has no UID.

`modeldev-step-v1` includes namespaced `pods get`: the pinned KFP driver reads
its own Pod when publishing its result, and the managed step CLI reads its own
Pod during bootstrap. This adds no Pod list/watch/write, exec, TokenRequest or
cluster-wide Namespace access. Each environment's existing RoleBinding limits
the ClusterRole to that tenant Namespace. This is the actual step call required
by `contracts/kfp-rbac.md` §4; the original spec §10.3 verb list omitted this read.

The workload guard protects original templates/parameters and Pod specs, permits
the exact KFP API terminate deadline of zero, and preserves status updates by
verified controllers/nodes. Legacy token Secrets for all three reserved SAs are
denied in every Namespace, including annotation-removal UPDATE attempts.

`native_probe.py` consumes explicit audited CREATE/UPDATE cases and an existing
real principal kubeconfig. It verifies the username and complete original groups
and only runs server dry-run. It issues no TokenRequest and uses no impersonation.
An RBAC denial cannot satisfy a ModelDev admission-denial assertion. Rendering
and component tests do not establish API-server enforcement or LIVE acceptance.

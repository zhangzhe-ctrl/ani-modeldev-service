#!/usr/bin/env python3
"""Sole-entry read-only identity/original RBAC proof for the finite Pod plan."""
import base64
import datetime
import getpass
import hashlib
import json
import os
import socket
import subprocess
import yaml

CTX = 'kubernetes-admin@ani-kcn-full'
SA = 'modeldev-control'
UID = '7100a777-b770-4f77-9509-631c8b39f1ec'

def require(ok, reason):
    if not ok:
        raise RuntimeError(reason)

def get(*args):
    p = subprocess.run(['kubectl', '--context', CTX, '--request-timeout=10s', *args, '-o', 'json'], capture_output=True, timeout=20)
    require(p.returncode == 0, 'EXACT_KUBE_READ_FAILED')
    return json.loads(p.stdout)

def run():
    require(getpass.getuser() == 'kubercloud' and os.environ.get('SSH_CONNECTION') and socket.gethostname() == 'node1', 'SOLE_SSH_IDENTITY_MISMATCH')
    cfg = get('config', 'view', '--minify')
    require(cfg['current-context'] == CTX and cfg['clusters'][0]['cluster']['server'] == 'https://lb.kubesphere.local:6443', 'CONTEXT_API_MISMATCH')
    ns = get('get', 'namespace', 'kube-system')
    require(ns['metadata']['uid'] == '3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24', 'CLUSTER_UID_MISMATCH')
    rp = get('-n', 'ani-system', 'get', 'secret', 'ani-resource-private')
    cluster = yaml.safe_load(base64.b64decode(rp['data']['config.yaml']))['network']['cluster_id']
    require(cluster == 'f39bf6e2-f3ed-4191-ada3-caeebf86c220', 'CLUSTER_ID_MISMATCH')
    target = get('get', 'namespace', 'ani-modeldev-tenant-a-v21')
    require(target['metadata']['uid'] == '94f3afc0-7548-4099-9e11-3fa7fdf6cc67', 'ORIGINAL_NAMESPACE_MISMATCH')
    sa = get('-n', 'ani-system', 'get', 'sa', SA)
    require(sa['metadata']['uid'] == UID, 'ORIGINAL_SA_UID_MISMATCH')
    dep = get('-n', 'ani-system', 'get', 'deployment', 'ani-modeldev')
    require(dep['metadata']['uid'] == 'fc5ecff7-f2a7-4d1d-9a0e-6f2f1d4366a3' and dep['spec']['replicas'] == 0, 'OLD_WORKER_NOT_STOPPED')
    selector = ','.join(k + '=' + v for k, v in sorted(dep['spec']['selector']['matchLabels'].items()))
    pods = get('-n', 'ani-system', 'get', 'pods', '-l', selector)['items']
    require(not pods, 'OLD_WORKER_POD_PRESENT')
    svc = get('-n', 'default', 'get', 'service', 'kubernetes')
    require(svc['spec']['clusterIP'] == '10.96.0.1' and any(p['port'] == 443 for p in svc['spec']['ports']), 'ORIGINAL_API_SERVICE_MISMATCH')
    db = get('-n', 'ani-platform', 'get', 'service', 'postgresql')
    require(db['metadata']['uid'] == 'a211aa89-9521-4570-836f-b9dbb1755e79' and db['spec']['clusterIP'] == '10.96.133.128' and any(p['port'] == 5432 for p in db['spec']['ports']), 'ORIGINAL_DATABASE_SERVICE_MISMATCH')
    q = "BEGIN READ ONLY; SET LOCAL statement_timeout='10s'; SELECT json_build_object('readonly',current_setting('transaction_read_only'),'leases',count(*)) FROM sys_modeldev_acceptances WHERE lease_until>clock_timestamp(); COMMIT;"
    r = subprocess.run(['kubectl', '--context', CTX, '--request-timeout=10s', '-n', 'ani-platform', 'exec', '-i', 'postgresql-0', '--', 'sh', '-c', 'PGPASSWORD="${POSTGRES_PASSWORD:-${POSTGRES_POSTGRES_PASSWORD:-}}" PGOPTIONS="-c default_transaction_read_only=on" exec psql -XqAt -U postgres -d ani_governance -v ON_ERROR_STOP=1'], input=q.encode(), capture_output=True, timeout=20)
    require(r.returncode == 0, 'CREATE_LEASE_READ_FAILED')
    leases = json.loads(r.stdout)
    require(leases['readonly'] == 'on' and leases['leases'] == 0, 'CREATE_LEASES_NOT_DRAINED')
    groups = {'system:authenticated', 'system:serviceaccounts', 'system:serviceaccounts:ani-system'}
    bindings = []
    for kind in ('rolebindings', 'clusterrolebindings'):
        args = ('get', kind, '-A') if kind == 'rolebindings' else ('get', kind)
        for b in get(*args)['items']:
            matches = [s for s in b.get('subjects', []) if s.get('kind') == 'ServiceAccount' and s.get('name') == SA and s.get('namespace') == 'ani-system' or s.get('kind') == 'Group' and s.get('name') in groups or s.get('kind') == 'User' and s.get('name') == 'system:serviceaccount:ani-system:' + SA]
            if not matches:
                continue
            ref = b['roleRef']
            roleargs = ('-n', b['metadata']['namespace'], 'get', ref['kind'], ref['name']) if ref['kind'] == 'Role' else ('get', ref['kind'], ref['name'])
            r = get(*roleargs)
            bindings.append({'kind': b['kind'], 'namespace': b['metadata'].get('namespace'), 'name': b['metadata']['name'], 'uid': b['metadata']['uid'], 'resource_version': b['metadata']['resourceVersion'], 'matching_subjects': matches, 'role_ref': ref, 'role_uid': r['metadata']['uid'], 'role_resource_version': r['metadata']['resourceVersion'], 'rules': r.get('rules', [])})
    return {'schema': 'ani.modeldev.original-abe-transport-preflight.v1', 'observed_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'identity': {'ssh': 'kubercloud@100.64.128.12', 'host': 'node1', 'context': CTX, 'apiserver': cfg['clusters'][0]['cluster']['server'], 'cluster_id': cluster, 'kube_system_uid': ns['metadata']['uid'], 'namespace_uid': target['metadata']['uid'], 'service_account_uid': UID, 'api_service_uid': svc['metadata']['uid'], 'api_service_vip': svc['spec']['clusterIP'], 'api_service_port': 443, 'old_worker_replicas': 0, 'old_worker_pods': 0, 'governance_create_leases': 0, 'database': 'ani_modeldev_v21', 'role': 'modeldev_v21_runtime', 'postgresql_service_uid': db['metadata']['uid'], 'postgresql_service_vip': db['spec']['clusterIP'], 'postgresql_service_port': 5432}, 'existing_original_bindings': bindings, 'cluster_mutation': False, 'new_rbac': False, 'new_sa': False, 'token_request': False}

if __name__ == '__main__':
    print(json.dumps(run(), indent=2))

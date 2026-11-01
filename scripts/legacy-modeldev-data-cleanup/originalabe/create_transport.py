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
    return {'schema': 'ani.modeldev.original-abe-transport-preflight.v1', 'observed_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'identity': {'ssh': 'kubercloud@100.64.128.12', 'host': 'node1', 'context': CTX, 'apiserver': cfg['clusters'][0]['cluster']['server'], 'cluster_id': cluster, 'kube_system_uid': ns['metadata']['uid'], 'namespace_uid': target['metadata']['uid'], 'service_account_uid': UID, 'api_service_uid': svc['metadata']['uid'], 'api_service_vip': svc['spec']['clusterIP'], 'api_service_port': 443, 'old_worker_replicas': 0, 'old_worker_pods': 0}, 'existing_original_bindings': bindings, 'cluster_mutation': False, 'new_rbac': False, 'new_sa': False, 'token_request': False}


EXPECTED_BINDINGS='58f11ec29aaa72d01b172d4114347f2e673caa5a0be45bbbfd1b042e4d3d99e6'
MANIFEST={'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': 'modeldev-original-abe-observer-20261101', 'namespace': 'ani-system', 'labels': {'ani.io/task': 'modeldev-tenantenv-20261101', 'ani.io/purpose': 'original-abe-absence-observer'}}, 'spec': {'nodeName': 'node1', 'serviceAccountName': 'modeldev-control', 'automountServiceAccountToken': True, 'restartPolicy': 'Never', 'activeDeadlineSeconds': 1800, 'securityContext': {'runAsNonRoot': True, 'runAsUser': 10001, 'runAsGroup': 10001, 'fsGroup': 10001, 'seccompProfile': {'type': 'RuntimeDefault'}}, 'containers': [{'name': 'transport', 'image': '10.10.1.66:5001/library/busybox@sha256:7a3ebe5bfd1a4a19797d20b0c0bb39d44393e9a03fd852c0865b0f540d868df0', 'command': ['/bin/sh', '-c', 'exec sleep 1800'], 'securityContext': {'allowPrivilegeEscalation': False, 'readOnlyRootFilesystem': True, 'capabilities': {'drop': ['ALL']}}}]}}
current=run()
require(hashlib.sha256(json.dumps(current['existing_original_bindings'],sort_keys=True,separators=(',',':')).encode()).hexdigest()==EXPECTED_BINDINGS,'ORIGINAL_BINDINGS_DRIFT')
r=subprocess.run(['kubectl','--context',CTX,'--request-timeout=10s','-n','ani-system','get','pod',MANIFEST['metadata']['name'],'-o','json'],capture_output=True,timeout=20)
require(r.returncode!=0 and b'NotFound' in r.stderr,'SAME_NAME_NOT_FOUND_REQUIRED')
r=subprocess.run(['kubectl','--context',CTX,'--request-timeout=10s','create','-f','-','-o','json'],input=json.dumps(MANIFEST).encode(),capture_output=True,timeout=20)
require(r.returncode==0,'EXACT_TRANSPORT_CREATE_FAILED')
p=json.loads(r.stdout)
require(p['spec']['serviceAccountName']==SA and p['spec']['nodeName']=='node1','CREATED_TRANSPORT_IDENTITY_MISMATCH')
print(json.dumps({'observed_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'identity':current['identity'],'original_binding_sha256':EXPECTED_BINDINGS,'pod':{'namespace':p['metadata']['namespace'],'name':p['metadata']['name'],'uid':p['metadata']['uid'],'resource_version':p['metadata']['resourceVersion'],'service_account_name':p['spec']['serviceAccountName'],'service_account_uid':UID,'node':p['spec']['nodeName'],'automount_service_account_token':p['spec']['automountServiceAccountToken'],'projected_sources':[v['projected']['sources'] for v in p['spec']['volumes'] if 'projected' in v]},'manifest_sha256':hashlib.sha256((json.dumps(MANIFEST,indent=2)+'\n').encode()).hexdigest(),'purpose':'single original ABE read-only plan; no worker/KFP/logs/observe mutation','cluster_mutation':'created finite transport Pod only','limits_present':False},indent=2))

#!/usr/bin/env python3
"""Prepare the finite CLI's original projected transport; no lifecycle writes."""
import base64
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

# preflight.py is sent as a fixed ordinary file beside this script.
from preflight import get, require, run, CTX, UID

ROOT = Path('/home/kubercloud/tenantenv-20261101/originalabe-20261101')
POD = 'modeldev-original-abe-observer-20261101'
POD_UID = 'ce8454b8-4d59-4d84-a7e0-a056fbe2d241'

def exclusive(name, raw):
    with (ROOT / name).open('xb') as f:
        os.chmod(f.name, 0o600)
        f.write(raw)

proof = run()
suffix = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')
pod = get('-n', 'ani-system', 'get', 'pod', POD)
require(pod['metadata']['uid'] == POD_UID and pod['spec']['nodeName'] == 'node1' and pod['spec']['serviceAccountName'] == 'modeldev-control' and not pod['metadata'].get('deletionTimestamp'), 'ORIGINAL_TRANSPORT_POD_MISMATCH')
require(pod['status']['phase'] == 'Running' and len(pod['status'].get('containerStatuses', [])) == 1 and pod['status']['containerStatuses'][0].get('ready'), 'TRANSPORT_NOT_RUNNING')
require(pod['spec'].get('automountServiceAccountToken') is True, 'NORMAL_API_PROJECTION_REQUIRED')
ROOT.mkdir(mode=0o700, parents=True, exist_ok=True)
require(ROOT.is_dir() and not ROOT.is_symlink(), 'PRIVATE_ROOT_INVALID')
os.chmod(ROOT, 0o700)
for source, dest in (('token', 'kubernetes-' + suffix + '.token'), ('ca.crt', 'kubernetes-ca-' + suffix + '.crt')):
    r = subprocess.run(['kubectl', '--context', CTX, '--request-timeout=10s', '-n', 'ani-system', 'exec', POD, '-c', 'transport', '--', 'cat', '/var/run/secrets/kubernetes.io/serviceaccount/' + source], capture_output=True, timeout=20)
    require(r.returncode == 0 and r.stdout, 'EXACT_PROJECTION_READ_FAILED')
    if source == 'token':
        parts = r.stdout.strip().decode().split('.')
        require(len(parts) == 3, 'PROJECTED_JWT_INVALID')
        claims = json.loads(base64.urlsafe_b64decode(parts[1] + '=' * (-len(parts[1]) % 4)))
        k = claims['kubernetes.io']
        require(claims['sub'] == 'system:serviceaccount:ani-system:modeldev-control' and k['serviceaccount']['uid'] == UID and k['pod']['uid'] == POD_UID, 'PROJECTED_IDENTITY_MISMATCH')
    exclusive(dest, r.stdout)
identity = proof['identity']
identity['observed_at'] = proof['observed_at']
identity['transport_pod_uid'] = POD_UID
raw = (json.dumps(identity, indent=2) + '\n').encode()
exclusive('identity-' + suffix + '.json', raw)
config = {'schema': 'ani.modeldev.original-abe-config.v1', 'dsn_file': '/home/kubercloud/tenantenv-20261101/legacy-cleanup-1464b131/private/modeldev-readonly.dsn', 'ca_file': str(ROOT / ('kubernetes-ca-' + suffix + '.crt')), 'token_file': str(ROOT / ('kubernetes-' + suffix + '.token')), 'endpoint': 'https://kubernetes.default.svc:443', 'server_name': 'kubernetes.default.svc', 'dial_address': '10.96.0.1:443', 'identity_file': str(ROOT / ('identity-' + suffix + '.json')), 'identity_sha256': hashlib.sha256(raw).hexdigest()}
raw = (json.dumps(config, indent=2) + '\n').encode()
exclusive('config-' + suffix + '.json', raw)
print(json.dumps({'observed_at': identity['observed_at'], 'transport_pod_uid': POD_UID, 'original_sa_uid': UID, 'config_path': str(ROOT / ('config-' + suffix + '.json')), 'config_sha256': hashlib.sha256(raw).hexdigest(), 'identity': identity, 'private_file_modes': '0600', 'private_root_mode': '0700', 'credentials_saved_in_evidence': False, 'database_mutation': False, 'kubernetes_mutation': False}, indent=2))

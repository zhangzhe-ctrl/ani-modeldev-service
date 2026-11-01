"""External boundary: one bounded SSH channel, no alternative host or retry."""

import json
import os
import selectors
import shlex
import subprocess
import time
import urllib.parse

from cleanup import ENTRY, Refusal

# All tokens, AWS credentials, response bodies and temporary artifact bytes stay
# on the sole approved node. Only selected identity/objects/hashes cross SSH.
NODE = r'''
import base64,hashlib,json,os,re,ssl,stat,subprocess,sys,tempfile,time,urllib.error,urllib.parse,urllib.request
class Failure(Exception):pass
def protected(path,limit):
 s=os.lstat(path)
 if not os.path.isabs(path) or not stat.S_ISREG(s.st_mode) or s.st_mode&0o077 or s.st_size>limit:raise Failure('NODE_PRIVATE_FILE_INVALID')
 return open(path,'rb').read()
def call(args,body=None):
 p=subprocess.run(args,input=body,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=25)
 if len(p.stdout)>16<<20:raise Failure('NODE_RESPONSE_LIMIT')
 return p
def kubectl(args,body=None):return call(['kubectl','--request-timeout=15s']+args,body)
def kube(method,path,body=None):
 if not path.startswith(('/api/v1/','/apis/')) or '\n' in path:raise Failure('NODE_KUBE_PATH_INVALID')
 p=kubectl(['get','--raw',path] if method=='GET' else ['delete','--raw',path,'-f','-'],None if body is None else json.dumps(body).encode())
 if p.returncode:
  if method=='GET' and p.stderr.startswith(b'Error from server (NotFound):'):return None
  raise Failure('KUBERNETES_READ_OR_CAS_DELETE_FAILED')
 return json.loads(p.stdout)
def identity():
 def get(args):
  p=kubectl(args)
  if p.returncode:raise Failure('FULL_IDENTITY_READ_FAILED')
  return p.stdout
 context=get(['config','current-context']).decode().strip()
 config=json.loads(get(['config','view','--minify','-o','json']))
 if len(config['clusters'])!=1:raise Failure('FULL_IDENTITY_DIFFERS')
 system=json.loads(get(['get','ns','kube-system','-o','json']))
 namespace=json.loads(get(['get','ns','ani-modeldev-tenant-a-v21','-o','json']))
 secret=json.loads(get(['-n','ani-system','get','secret','ani-resource-private','-o','json']))
 source=base64.b64decode(secret['data']['config.yaml'])
 found=re.findall(rb'(?m)^\s*cluster_id:\s*[\x22\x27]?([a-f0-9-]{36})',source)
 if len(found)!=1 or namespace['metadata']['uid']!='94f3afc0-7548-4099-9e11-3fa7fdf6cc67' or namespace['metadata'].get('deletionTimestamp'):raise Failure('FULL_IDENTITY_DIFFERS')
 return {'ssh':'kubercloud@100.64.128.12','context':context,'api_server':config['clusters'][0]['cluster']['server'],'cluster_id':found[0].decode(),'kube_system_uid':system['metadata']['uid']}
def kfp(c,method,path):
 endpoint=c['endpoint']
 parsed=urllib.parse.urlsplit(endpoint)
 if parsed.scheme!='https' or not parsed.hostname or parsed.username or parsed.query or parsed.fragment:raise Failure('KFP_CONNECTION_INVALID')
 token=protected(c['token_file'],16384).decode().strip()
 claims=json.loads(base64.urlsafe_b64decode(token.split('.')[1]+'==='))
 if claims.get('sub')!='system:serviceaccount:ani-system:modeldev-control' or claims.get('aud')!=['pipelines.kubeflow.org'] or claims.get('exp',0)<=time.time()+10 or claims.get('kubernetes.io',{}).get('pod',{}).get('uid')!=c['control_pod_uid']:raise Failure('ORIGINAL_KFP_CONTROL_IDENTITY_DIFFERS')
 class NoRedirect(urllib.request.HTTPRedirectHandler):
  def redirect_request(self,*args,**kwargs):return None
 opener=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=c['ca_file'])),NoRedirect())
 req=urllib.request.Request(endpoint.rstrip('/')+path,method=method,headers={'Authorization':'Bearer '+token,'Accept':'application/json'})
 try:
  with opener.open(req,timeout=20) as response:raw=response.read((16<<20)+1)
 except urllib.error.HTTPError as e:
  if method=='GET' and e.code==404:return None
  raise Failure('KFP_AUTH_OR_HTTP_FAILED')
 if len(raw)>16<<20:raise Failure('KFP_RESPONSE_LIMIT')
 return json.loads(raw or b'{}')
def s3(c,operation,target):
 for key in ('credentials_file','config_file'):protected(c[key],1<<20)
 if not c['endpoint'].startswith('https://'):raise Failure('S3_TLS_REQUIRED')
 env=os.environ.copy()
 for key in list(env):
  if key.startswith('AWS_'):del env[key]
 env.update(AWS_SHARED_CREDENTIALS_FILE=c['credentials_file'],AWS_CONFIG_FILE=c['config_file'],AWS_EC2_METADATA_DISABLED='true',AWS_MAX_ATTEMPTS='1')
 prefix=['aws','--no-cli-pager','--profile',c['profile'],'--endpoint-url',c['endpoint'],'--ca-bundle',c['ca_file'],'--cli-connect-timeout','10','--cli-read-timeout','20','s3api']
 suffix=['--bucket',target['bucket'],'--key',target['key'],'--version-id',target['version_id']]
 def invoke(args):
  p=subprocess.run(prefix+args,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=25)
  if len(p.stdout)>1<<20:raise Failure('S3_METADATA_LIMIT')
  return p
 if operation=='DELETE_VERSION':
  p=invoke(['delete-object']+suffix)
  if p.returncode:raise Failure('S3_VERSION_DELETE_UNCONFIRMED')
  return {}
 p=invoke(['head-object']+suffix)
 if p.returncode:
  if any(s in p.stderr for s in (b'(404)',b'(NoSuchVersion)',b'(NoSuchKey)')):return None
  raise Failure('S3_VERSION_HEAD_FAILED')
 head=json.loads(p.stdout)
 if head.get('VersionId')!=target['version_id'] or head.get('ContentLength',-1)>64<<20:raise Failure('S3_IMMUTABLE_VERSION_DIFFERS')
 fd,path=tempfile.mkstemp(prefix='modeldev-original-version-')
 os.close(fd)
 try:
  p=invoke(['get-object']+suffix+[path])
  if p.returncode or os.path.getsize(path)>64<<20:raise Failure('S3_ORIGINAL_VERSION_BYTES_UNVERIFIED')
  digest=hashlib.sha256()
  with open(path,'rb') as handle:
   for block in iter(lambda:handle.read(1<<20),b''):digest.update(block)
  return {'version_id':head['VersionId'],'etag':head['ETag'],'size_bytes':head['ContentLength'],'sha256':digest.hexdigest()}
 finally:os.unlink(path)
for line in sys.stdin:
 try:
  request=json.loads(line)
  operation=request['op']
  if operation=='identity':value=identity()
  elif operation=='kube':value=kube(request['method'],request['path'],request.get('body'))
  elif operation=='kfp':value=kfp(request['connection'],request['method'],request['path'])
  elif operation=='s3':value=s3(request['connection'],request['operation'],request['target'])
  else:raise Failure('NODE_OPERATION_INVALID')
  print(json.dumps({'ok':value},separators=(',',':')),flush=True)
 except Exception as error:
  print(json.dumps({'error':str(error) if isinstance(error,Failure) else type(error).__name__}),flush=True)
'''


class SSHPeer:
    def __init__(self, connection):
        if set(connection) != {"kfp", "s3"}:
            raise Refusal("CONNECTION_FIELDS_INVALID")
        if set(connection["kfp"]) != {"endpoint", "ca_file", "token_file", "control_pod_uid"} or set(connection["s3"]) != {"endpoint", "ca_file", "credentials_file", "config_file", "profile"}:
            raise Refusal("CONNECTION_REFERENCES_INVALID")
        self.connection, self.process, self.buffer = connection, None, b""

    def request(self, message):
        if self.process is None:
            self.process = subprocess.Popen(["ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2", ENTRY, "python3 -u -c " + shlex.quote(NODE)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=False)
        if self.process.poll() is not None:
            raise Refusal("SOLE_SSH_CHANNEL_TERMINATED_NO_RETRY")
        self.process.stdin.write(json.dumps(message).encode() + b"\n")
        self.process.stdin.flush()
        selector = selectors.DefaultSelector()
        selector.register(self.process.stdout, selectors.EVENT_READ)
        try:
            deadline = time.monotonic() + 60
            while b"\n" not in self.buffer:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not selector.select(remaining):
                    self.process.kill()
                    raise Refusal("SOLE_SSH_TIMEOUT_RESULT_UNKNOWN_NO_RETRY")
                block = os.read(self.process.stdout.fileno(), 65536)
                if not block:
                    raise Refusal("SOLE_SSH_RESPONSE_UNAVAILABLE")
                self.buffer += block
                if len(self.buffer) > 16 << 20:
                    raise Refusal("SOLE_SSH_RESPONSE_LIMIT")
            raw, self.buffer = self.buffer.split(b"\n", 1)
        finally:
            selector.close()
        if not raw or len(raw) > 16 << 20:
            raise Refusal("SOLE_SSH_RESPONSE_UNAVAILABLE")
        result = json.loads(raw)
        if "error" in result:
            raise Refusal("NODE_" + result["error"])
        return result["ok"]

    def identity(self):
        return self.request({"op": "identity"})

    def kube(self, method, path, body=None):
        return self.request({"op": "kube", "method": method, "path": path, "body": body})

    def kube_list(self, path):
        items, continuation, seen = [], "", set()
        for _ in range(20):
            query = urllib.parse.urlencode({"limit": 200, "continue": continuation})
            page = self.kube("GET", path + "?" + query)
            if page is None or not isinstance(page.get("items"), list):
                raise Refusal("COMPLETE_KUBE_REFERENCES_UNVERIFIED")
            items.extend(page["items"])
            continuation = page.get("metadata", {}).get("continue", "")
            if not continuation:
                return items
            if continuation in seen:
                break
            seen.add(continuation)
        raise Refusal("KUBE_REFERENCE_PAGINATION_LIMIT")

    def kfp(self, method, path):
        return self.request({"op": "kfp", "method": method, "path": path, "connection": self.connection["kfp"]})

    def kfp_list(self, kind, filters=None):
        items, token, seen = [], "", set()
        field = "versions" if kind == "pipeline_versions" else kind
        for _ in range(20):
            query = dict(filters or {}, page_size=100, page_token=token)
            page = self.kfp("GET", "/apis/v1beta1/" + kind + "?" + urllib.parse.urlencode(query))
            if page is None or not isinstance(page, dict) or set(page) - {field, "total_size", "next_page_token"} or not isinstance(page.get(field, []), list):
                raise Refusal("COMPLETE_KFP_REFERENCES_UNVERIFIED")
            items.extend(page.get(field, []))
            token = page.get("next_page_token", "")
            if not token:
                if "total_size" in page and page["total_size"] != len(items):
                    raise Refusal("COMPLETE_KFP_REFERENCE_COUNT_DIFFERS")
                return items
            if token in seen:
                break
            seen.add(token)
        raise Refusal("KFP_REFERENCE_PAGINATION_LIMIT")

    def s3(self, operation, target):
        return self.request({"op": "s3", "operation": operation, "target": target, "connection": self.connection["s3"]})

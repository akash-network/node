import time
import re
import subprocess
import sys
import os
import tempfile
import json
import logging
import logging.config
import pickle 
import struct

TENANT_NAMESPACE_RE = re.compile('^[a-z,0-9]{45}$')

AKASH_LEASE_ID_PROVIDER_LABEL = 'akash.network/lease.id.provider'
AKASH_NETWORK_NAMESPACE_LABEL = 'akash.network/namespace'
AKASH_NETWORK_DSEQ_LABEL = 'akash.network/lease.id.dseq'
AKASH_NETWORK_OSEQ_LABEL = 'akash.network/lease.id.oseq'
AKASH_NETWORK_GSEQ_LABEL = 'akash.network/lease.id.gseq'
AKASH_NETWORK_OWNER_LABEL = 'akash.network/lease.id.owner'
AKASH_MANAGED_LABEL = 'akash.network'

def parse_labels(data):
  output = {}

  provider = data.get(AKASH_LEASE_ID_PROVIDER_LABEL)
  if provider is not None:
    output['provider'] = provider

  owner = data.get(AKASH_NETWORK_OWNER_LABEL)
  if owner is not None:
    output['owner'] = owner

  dseq = data.get(AKASH_NETWORK_DSEQ_LABEL)
  if dseq is not None:
    output['dseq'] = int(dseq)

  gseq = data.get(AKASH_NETWORK_GSEQ_LABEL)
  if gseq is not None:
    output['gseq'] = int(gseq)

  oseq = data.get(AKASH_NETWORK_OSEQ_LABEL)
  if oseq is not None:
    output['oseq'] = int(oseq)

  return output

def run_kubectl(*args, ojson = True, stdin_data = None):
  l = logging.getLogger('akash.kubectl')
  args = list(args)
  if ojson:
    args.append('--output=json')
  args = ['kubectl'] + args
  with tempfile.TemporaryFile() as tmp_stdout:
    with tempfile.TemporaryFile() as tmp_stderr:
      with tempfile.TemporaryFile() as tmp_stdin:
        stdin = subprocess.DEVNULL
        if stdin_data is not None:
          if type(stdin_data) == str:
            tmp_stdin.write(bytes(stdin_data, 'utf-8'))
          else:
            tmp_stdin.write(stdin_data)
          tmp_stdin.seek(0)
          stdin = tmp_stdin
        l.info("running %s", args)
        proc = subprocess.Popen(args, executable = 'kubectl', stdin = stdin, stdout = tmp_stdout, stderr = tmp_stderr)
        proc.communicate()
        rc = proc.wait()
        if rc != 0:
          tmp_stderr.seek(0)
          logging.error("kubectl stderr: " + str(tmp_stderr.read()).strip())
          raise RuntimeError("kubectl failed with %d" % (rc,))

        tmp_stdout.seek(0)
        if ojson:
          output_data = json.load(tmp_stdout)
        else:
          output_data = tmp_stdout.read()

  return output_data

def get_tenant_namespaces():
  l = logging.getLogger('akash.get_tenant_namespaces')
  result = run_kubectl('get', 'namespaces', '--selector=akash.network=true')
  output = []
  for item in result['items']:
    ns = item['metadata']['name']

    l.info('found namespace %s', ns)
    output.append((ns, parse_labels(item['metadata']['labels']),))
  return output

class ProviderHostData(object):
  def __init__(self, provider, owner, dseq, gseq, oseq, service_name, service_port, hostname, namespace):
    self.provider = provider
    self.owner = owner
    self.dseq = dseq
    self.gseq = gseq
    self.oseq = oseq
    self.service_name = service_name
    self.service_port = service_port
    self.hostname = hostname
    self.namespace = namespace
    
def append_to_datafile(fout, obj):
  data = pickle.dumps(obj)
  entry_header = struct.pack('>I', len(data))
  fout.write(entry_header)
  fout.write(data)

def read_from_datafile(fin):
  header = fin.read(4)
  if len(header) == 0:
    return None

  data_length, = struct.unpack('>I', header)
  data = fin.read(data_length)
  return pickle.loads(data)

PROVIDER_HOSTS_FILE_NAME = 'provider_hosts.pickle'

def remove_ingresses():
  done = set()
  with open(PROVIDER_HOSTS_FILE_NAME, 'rb') as fin:
    while True:
      phd = read_from_datafile(fin)
      if phd is None:
        break

      if phd.namespace not in done:
        run_kubectl('delete', 'ingress', phd.service_name, '--namespace=' + phd.namespace, ojson = False)
        done.add(phd.namespace)
 
def traverse_ingresses():
  provider_hosts_fout = open(PROVIDER_HOSTS_FILE_NAME, 'wb')
  ingress_backup_fout = open('ingresses_backup.json', 'w')
  l = logging.getLogger('akash.traverse_ingresses')
  for ns, ns_data in get_tenant_namespaces():
    l.info("Checking namespace %s for ingress resources", ns)
    result = run_kubectl('get', 'ingress', '--namespace=' + ns, '--selector=' + AKASH_NETWORK_NAMESPACE_LABEL)

    for item in result['items']:
      json.dump(item, ingress_backup_fout)
      ingress_backup_fout.write("\n\n")
      md = item['metadata']
      labels = md['labels']
      spec = item['spec']
      
      for rule in spec['rules']:
        hostname = rule['host']
        ingress_path = rule['http']['paths'][0]
        service = ingress_path['backend']['service']
        service_name = service['name']
        service_port = int(service['port']['number'])
        provider = ns_data['provider']
        dseq = ns_data['dseq']
        oseq = ns_data['oseq']
        gseq = ns_data['gseq']
        owner = ns_data['owner']
        logging.info("found existing ingress %s - %s - %d", hostname, service_name, service_port)
        pd = ProviderHostData(provider, owner, dseq, gseq, oseq, service_name, service_port, hostname, ns)
        append_to_datafile(provider_hosts_fout, pd)

  provider_hosts_fout.close()
  ingress_backup_fout.close()

def create_providerhosts():
  with open(PROVIDER_HOSTS_FILE_NAME, 'rb') as fin:
    while True:
      phd = read_from_datafile(fin)
      if phd is None:
        break

      crd_resource = {
        'apiVersion': 'akash.network/v1',
        'kind': 'ProviderHost',
        'metadata': {
           'name': phd.hostname,
           'labels': {
              AKASH_MANAGED_LABEL: 'true',
              AKASH_LEASE_ID_PROVIDER_LABEL: phd.provider,
              AKASH_NETWORK_DSEQ_LABEL: str(phd.dseq),
              AKASH_NETWORK_OSEQ_LABEL: str(phd.oseq),
              AKASH_NETWORK_GSEQ_LABEL: str(phd.gseq),
              AKASH_NETWORK_OWNER_LABEL: phd.owner,
            }
         },
        'spec': {
          'service_name': phd.service_name,
          'external_port': phd.service_port,
          'hostname': phd.hostname,
          'owner': phd.owner,
          'provider': phd.provider, 
          'dseq': phd.dseq,
          'gseq': phd.gseq,
          'oseq': phd.oseq,
        }
      }
      run_kubectl('apply', '-n', 'lease', '-f', '-', stdin_data  = json.dumps(crd_resource))


log_cfg = {
    'version': 1,
    'formatters': {
        'detailed': {
            'class': 'logging.Formatter',
            'format': '%(asctime)s %(name)-15s %(levelname)-8s %(message)s'
        }
    },
    'handlers': {
        'console': {
            'class': 'logging.StreamHandler',
            'level': 'DEBUG',
            'formatter': 'detailed',
        },
    },
    'loggers': {
        'akash': {
            'handlers': ['console'],
            'propagate': False,
        }
    },
    'root': {
        'level': 'DEBUG',
        'handlers': ['console']
    },
}

logging.config.dictConfig(log_cfg)

if len(sys.argv) < 2:
  sys.stderr.write("specify target on command line\n")
  sys.exit(1)

target = sys.argv[1]

if target == 'backup':
  traverse_ingresses()
elif target == 'create':
  create_providerhosts()
elif target == 'purge':
  remove_ingresses()
else: 
  sys.stderr.write("unknown target: " + target + "\n")
  sys.exit(1)



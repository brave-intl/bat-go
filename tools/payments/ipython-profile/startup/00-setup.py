#!/usr/bin/env python
import atexit
import base64
import builtins
import functools
import json
import os
import subprocess
import sys

from IPython.core.magic import register_line_magic
from IPython.core.magic_arguments import (argument, magic_arguments, parse_argstring)
from IPython.lib import backgroundjobs as bg
from IPython.terminal.prompts import Prompts, Token
from subprocess import DEVNULL

environment = "local"
cluster = ""
aws_vault = os.getenv("AWS_VAULT", "")
if "prod" in aws_vault:
    environment = "prod"
elif "stg" in aws_vault or "staging" in aws_vault:
    environment = "staging"
# intentionally put dev last since many roles have "developer" in them
elif "dev" in aws_vault:
    environment = "dev"
del aws_vault

# FIXME TODO pull env vars and set during pcr verification

secrets_s3_bucket = ""
pcr2 = ""
payout_id = "none"
prepare_log = ""
payout_report = ""
redis_username = os.getenv("REDIS_USERNAME")
redis_password = os.getenv("REDIS_PASSWORD")
operator_key = "~/.ssh/settlements"

jobs = bg.BackgroundJobManager()

def _set_cluster():
    global cluster
    if environment =="prod" or environment == "staging":
        cluster = "bsg-production"
    elif environment == "dev":
        cluster = "bsg-sandbox"
_set_cluster()

def _start_redis_forwarding():
    try:
        _set_cluster()
        print("starting redis port forwarding...")
        env = os.environ.copy()
        if "AWS_VAULT" in env:
            del env["AWS_VAULT"]
        p = subprocess.Popen(["kubectl", "--context", cluster, "--namespace", f"payment-{environment}", "port-forward", "service/redis-proxy", "6380:6379"], stdout=DEVNULL, stderr=DEVNULL, env=env)
        atexit.register(p.kill)
        p.wait()
    except Exception as e:
        print(e)

def _set_redis_credentials():
    global redis_username, redis_password
    env = os.environ.copy()
    if "AWS_VAULT" in env:
        del env["AWS_VAULT"]
    p = subprocess.Popen(["kubectl", "--context", cluster, "--namespace", f"payment-{environment}", "get", "secret", "redis-credentials", "-o", "json"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    output, _ = p.communicate()
    result = json.loads(output.decode('utf-8'))
    redis_username = base64.b64decode(result["data"]["operator_username"]).decode("utf-8")
    redis_password = base64.b64decode(result["data"]["operator_password"]).decode("utf-8")

def _get_web_env():
    global redis_username, redis_password
    env = os.environ.copy()
    if "AWS_VAULT" in env:
        del env["AWS_VAULT"]
    p = subprocess.Popen(["kubectl", "--context", cluster, "--namespace", f"payment-{environment}", "get", "deployments", "web", "-o", "json"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    output, _ = p.communicate()
    result = json.loads(output.decode('utf-8'))
    env = {}
    for container in result["spec"]["template"]["spec"]["containers"]:
        if container["name"] == "nitro-shim":
            for var in container["env"]:
                if "name" in var and "value" in var:
                    env[var["name"]] = var["value"]
    return env

def _set_secrets_s3_bucket():
    global secrets_s3_bucket 
    env = _get_web_env()
    secrets_s3_bucket = env["ENCLAVE_CONFIG_BUCKET_NAME"]

def _get_pcr2():
    cwd = subprocess.run(['git', 'rev-parse', '--show-toplevel'], stdout=subprocess.PIPE).stdout.decode('utf-8').strip()
    p = subprocess.Popen(["make", "pcrs"], cwd=cwd, stdout=subprocess.PIPE)
    lines = []
    for line in p.stdout:
        line = line.decode("utf-8")
        print(line, end='', flush=True)
        lines.append(line)

    measurement = json.loads(lines[-1].split(":", 2)[-1])
    return measurement["PCR2"]

if environment != "local":
    jobs.new(_start_redis_forwarding, daemon=True)
    _set_redis_credentials()
    _set_secrets_s3_bucket()

@register_line_magic
def _redis_credentials(self):
    _set_redis_credentials()

@register_line_magic
def _redis_proxy(self):
    jobs.new(_start_redis_forwarding, daemon=True)

@register_line_magic
def _set_pcr2(self):
    global pcr2
    pcr2 = _get_pcr2()

@magic_arguments()
@argument('fn', type=str, help='The payout report filename.')
@register_line_magic
def _payout(fn):
    """ Prepare to payout the report specified. """
    global environment, payout_id, payout_report, prepare_log

    with builtins.open(os.path.expanduser(fn)) as f:
        payout_report = fn
        payments = json.load(f)
        total = functools.reduce(lambda x, y: x + y['amount'], payments, 0)
        custodians = functools.reduce(lambda x, y: (x.add(y['custodian']), x)[-1], payments, set())
        currency = payments[0]['currency']
        print(f"{len(payments)} payments for a total of {total} {currency}")
        print(f"custodians: {', '.join(custodians)}")
        payout_id = payments[0]['payoutId']
        prepare_log = f"prepare-{payout_id}-response.log"

class SettlementPrompt(Prompts):
     def in_prompt_tokens(self):
         return [(Token, f"{environment}:{payout_id}"),
                 (Token.Prompt, ' >>> ')]

ip = get_ipython()
ip.prompts = SettlementPrompt(ip)

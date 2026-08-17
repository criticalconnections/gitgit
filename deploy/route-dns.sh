#!/usr/bin/env bash
# Create the DNS record pointing a hostname at the GitGit tunnel.
#
# Why this script exists — `cloudflared tunnel route dns` has two footguns that
# will silently write a record to the WRONG zone and the WRONG tunnel:
#
#   1. `cloudflared tunnel login` prints an error and still exits 0 when a
#      cert.pem already exists, so `login && route dns` runs anyway.
#   2. `route dns` ignores the tunnel-name argument and uses the `tunnel:` key
#      from ~/.cloudflared/config.yml, while resolving the hostname relative to
#      whatever zone cert.pem is scoped to. Asking for "gitgit.io" while the
#      cert is scoped to example.com creates "gitgit.io.example.com".
#
# This script checks both before writing anything.
#
#   usage: deploy/route-dns.sh <hostname> [tunnel-uuid]

set -euo pipefail

HOSTNAME_ARG="${1:-}"
TUNNEL_UUID="${2:-17ea65be-74eb-4b18-93b1-27f081806598}"
CERT="${HOME}/.cloudflared/cert.pem"

if [[ -z "$HOSTNAME_ARG" ]]; then
  echo "usage: $0 <hostname> [tunnel-uuid]" >&2
  exit 64
fi
[[ -f "$CERT" ]] || { echo "no cert at $CERT — run 'cloudflared tunnel login' first" >&2; exit 1; }

# Which zone is the current cert actually scoped to?
read -r CERT_ZONE_ID CERT_ACCOUNT <<<"$(python3 - "$CERT" <<'PY'
import base64, json, re, sys
raw = open(sys.argv[1]).read()
m = re.search(r'-----BEGIN ARGO TUNNEL TOKEN-----(.*?)-----END ARGO TUNNEL TOKEN-----', raw, re.S)
d = json.loads(base64.b64decode(re.sub(r'\s+', '', m.group(1))))
print(d.get("zoneID", ""), d.get("accountID", ""))
PY
)"

ZONE_NAME=$(python3 - "$CERT" "$CERT_ZONE_ID" <<'PY'
import base64, json, re, sys, urllib.request
raw = open(sys.argv[1]).read()
m = re.search(r'-----BEGIN ARGO TUNNEL TOKEN-----(.*?)-----END ARGO TUNNEL TOKEN-----', raw, re.S)
tok = json.loads(base64.b64decode(re.sub(r'\s+', '', m.group(1))))["apiToken"]
req = urllib.request.Request(f"https://api.cloudflare.com/client/v4/zones/{sys.argv[2]}",
                             headers={"Authorization": "Bearer " + tok})
try:
    with urllib.request.urlopen(req) as r:
        print(json.load(r)["result"]["name"])
except Exception:
    print("")
PY
)

if [[ -z "$ZONE_NAME" ]]; then
  echo "could not determine the zone this cert is scoped to" >&2
  exit 1
fi

# The whole point: refuse when the cert cannot write the requested hostname.
if [[ "$HOSTNAME_ARG" != "$ZONE_NAME" && "$HOSTNAME_ARG" != *".$ZONE_NAME" ]]; then
  cat >&2 <<EOF
REFUSING: this cert is scoped to the "$ZONE_NAME" zone, but you asked for
"$HOSTNAME_ARG". Running cloudflared here would create the bogus record
"$HOSTNAME_ARG.$ZONE_NAME" on $ZONE_NAME instead of failing.

To route $HOSTNAME_ARG, do ONE of:

  a) re-authenticate for that zone, preserving the current cert:
       mv ~/.cloudflared/cert.pem ~/.cloudflared/cert.$ZONE_NAME.pem
       cloudflared tunnel login          # pick $HOSTNAME_ARG in the browser
       $0 $HOSTNAME_ARG $TUNNEL_UUID

  b) add the record by hand in the Cloudflare dashboard for $HOSTNAME_ARG:
       Type:   CNAME
       Name:   @        (or the subdomain)
       Target: $TUNNEL_UUID.cfargotunnel.com
       Proxy:  Proxied (orange cloud) — required

Note: the tunnel and the zone must live in the SAME Cloudflare account, or
the cfargotunnel.com target will not resolve.
EOF
  exit 2
fi

echo "cert zone:  $ZONE_NAME"
echo "hostname:   $HOSTNAME_ARG"
echo "tunnel:     $TUNNEL_UUID"
echo

# Write the CNAME through the API rather than `cloudflared tunnel route dns`,
# so the global config's `tunnel:` key cannot hijack which tunnel we point at.
python3 - "$CERT" "$CERT_ZONE_ID" "$HOSTNAME_ARG" "$TUNNEL_UUID" <<'PY'
import base64, json, re, sys, urllib.request, urllib.error
cert, zone, host, uuid = sys.argv[1:5]
raw = open(cert).read()
m = re.search(r'-----BEGIN ARGO TUNNEL TOKEN-----(.*?)-----END ARGO TUNNEL TOKEN-----', raw, re.S)
tok = json.loads(base64.b64decode(re.sub(r'\s+', '', m.group(1))))["apiToken"]

def api(method, path, body=None):
    req = urllib.request.Request("https://api.cloudflare.com/client/v4" + path, method=method,
        headers={"Authorization": "Bearer " + tok, "Content-Type": "application/json"},
        data=json.dumps(body).encode() if body else None)
    try:
        with urllib.request.urlopen(req) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        return json.load(e)

target = f"{uuid}.cfargotunnel.com"
existing = api("GET", f"/zones/{zone}/dns_records?name={host}").get("result", [])
body = {"type": "CNAME", "name": host, "content": target, "proxied": True}

if existing:
    rec = existing[0]
    if rec["content"] == target:
        print(f"already correct: {host} -> {target}")
        raise SystemExit(0)
    print(f"updating existing {rec['type']} {host} ({rec['content']} -> {target})")
    out = api("PATCH", f"/zones/{zone}/dns_records/{rec['id']}", body)
else:
    print(f"creating CNAME {host} -> {target}")
    out = api("POST", f"/zones/{zone}/dns_records", body)

if out.get("success"):
    print("ok")
else:
    print("FAILED:", [e.get("message") for e in out.get("errors", [])])
    raise SystemExit(1)
PY

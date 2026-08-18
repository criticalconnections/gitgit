#!/usr/bin/env bash
# Provision one GitGit customer instance on Hetzner Cloud.
#
#   deploy/provision-customer.sh acme                 # what it would do
#   deploy/provision-customer.sh acme --run           # actually do it
#
# Creates the server with a cloud-init payload, points DNS at it, sets reverse
# DNS to the customer's own hostname so nothing in a traceroute says Hetzner,
# and waits for /healthz before declaring success.
#
# Deliberately dry-run by default: this spends money and creates public DNS.
set -euo pipefail

CUSTOMER="${1:-}"
RUN="${2:-}"
[ -z "$CUSTOMER" ] && { echo "usage: $0 <customer-slug> [--run]"; exit 2; }
[[ "$CUSTOMER" =~ ^[a-z0-9][a-z0-9-]{1,30}$ ]] || { echo "slug must be lowercase letters, digits and dashes"; exit 2; }

: "${HCLOUD_TOKEN:?set HCLOUD_TOKEN (Hetzner Cloud console -> Security -> API tokens, Read & Write)}"
: "${CF_API_TOKEN:?set CF_API_TOKEN (Cloudflare token with Zone:DNS:Edit on the base domain)}"
: "${CF_ZONE_ID:?set CF_ZONE_ID for the base domain}"

BASE_DOMAIN="${BASE_DOMAIN:-gitgit.io}"
SERVER_TYPE="${SERVER_TYPE:-cx33}"       # 4 vCPU / 8 GB / 80 GB / 20 TB
LOCATION="${LOCATION:-hel1}"             # hel1 Finland, fsn1 Germany, ash US-East
IMAGE="${IMAGE:-ubuntu-24.04}"
GITGIT_IMAGE="${GITGIT_IMAGE:-ghcr.io/criticalconnections/gitgit:latest}"
ACME_EMAIL="${ACME_EMAIL:-admin@$BASE_DOMAIN}"
SSH_KEY_NAME="${SSH_KEY_NAME:-gitgit-ops}"

HOSTNAME="$CUSTOMER.$BASE_DOMAIN"
HAPI="https://api.hetzner.cloud/v1"
CAPI="https://api.cloudflare.com/client/v4"

h()  { curl -fsS -H "Authorization: Bearer $HCLOUD_TOKEN" -H "Content-Type: application/json" "$@"; }
cf() { curl -fsS -H "Authorization: Bearer $CF_API_TOKEN" -H "Content-Type: application/json" "$@"; }
say(){ printf "\033[1;32m==>\033[0m %s\n" "$*"; }

# A per-instance key so one customer's ciphertext never opens with another's.
SECRET_KEY="$(openssl rand -hex 32)"

USER_DATA="$(sed \
  -e "s|__HOSTNAME__|$HOSTNAME|g" \
  -e "s|__CUSTOMER__|$CUSTOMER|g" \
  -e "s|__IMAGE__|$GITGIT_IMAGE|g" \
  -e "s|__ACME_EMAIL__|$ACME_EMAIL|g" \
  -e "s|__SECRET_KEY__|$SECRET_KEY|g" \
  -e "s|__CF_TOKEN__|$CF_API_TOKEN|g" \
  -e "s|__R2_ACCOUNT__|${GITGIT_R2_ACCOUNT_ID:-}|g" \
  -e "s|__R2_BUCKET__|${GITGIT_R2_BUCKET:-}|g" \
  -e "s|__R2_KEY_ID__|${GITGIT_R2_ACCESS_KEY_ID:-}|g" \
  -e "s|__R2_SECRET__|${GITGIT_R2_SECRET_ACCESS_KEY:-}|g" \
  "$(dirname "$0")/cloud-init.yaml.tmpl")"

if [ "$RUN" != "--run" ]; then
  cat <<PLAN
DRY RUN — nothing was created. Pass --run to execute.

  customer     $CUSTOMER
  hostname     $HOSTNAME
  server       $SERVER_TYPE in $LOCATION, image $IMAGE
  container    $GITGIT_IMAGE
  DNS          A $HOSTNAME  and  A *.preview.$HOSTNAME   (DNS-only, not proxied)
  reverse DNS  <ip> -> $HOSTNAME
  TLS          Caddy, Let's Encrypt via DNS-01 (covers the depth-2 wildcard)
  backups      every 24h, 3 local, 30 in R2 under prefix "$CUSTOMER/"
  secret key   generated per instance (shown once on success)

Estimated cost: $SERVER_TYPE is roughly EUR 8.49/month including 20 TB traffic.
PLAN
  exit 0
fi

say "Creating $SERVER_TYPE in $LOCATION for $CUSTOMER"
CREATE=$(h -X POST "$HAPI/servers" -d "$(python3 - <<PY
import json,os
print(json.dumps({
  "name": "gitgit-$CUSTOMER",
  "server_type": "$SERVER_TYPE",
  "image": "$IMAGE",
  "location": "$LOCATION",
  "ssh_keys": ["$SSH_KEY_NAME"],
  "start_after_create": True,
  "labels": {"product": "gitgit", "customer": "$CUSTOMER"},
  "user_data": os.environ["USER_DATA"],
}))
PY
)")
export USER_DATA
SERVER_ID=$(echo "$CREATE" | python3 -c "import sys,json;print(json.load(sys.stdin)['server']['id'])")
IPV4=$(echo "$CREATE" | python3 -c "import sys,json;print(json.load(sys.stdin)['server']['public_net']['ipv4']['ip'])")
say "server $SERVER_ID at $IPV4"

say "Pointing DNS at it (DNS-only: proxying would cap git pushes at 100 MB)"
for NAME in "$HOSTNAME" "*.preview.$HOSTNAME"; do
  cf -X POST "$CAPI/zones/$CF_ZONE_ID/dns_records" \
     -d "{\"type\":\"A\",\"name\":\"$NAME\",\"content\":\"$IPV4\",\"ttl\":120,\"proxied\":false}" >/dev/null
  echo "   A $NAME -> $IPV4"
done

say "Setting reverse DNS so the IP identifies as $HOSTNAME, not Hetzner"
h -X POST "$HAPI/servers/$SERVER_ID/actions/change_dns_ptr" \
  -d "{\"ip\":\"$IPV4\",\"dns_ptr\":\"$HOSTNAME\"}" >/dev/null

say "Waiting for the instance to answer /healthz (first boot installs Docker and issues certs)"
for i in $(seq 1 90); do
  if curl -fsS --max-time 5 "https://$HOSTNAME/healthz" >/dev/null 2>&1; then
    say "healthy after $((i*10))s"
    curl -fsS "https://$HOSTNAME/healthz" | python3 -m json.tool
    cat <<DONE

  $CUSTOMER is live at https://$HOSTNAME
  git over SSH:  git@$HOSTNAME:owner/repo.git
  admin SSH:     ssh -p 2299 root@$IPV4

  SECRET KEY (store this — backups are useless without it, and it is not in them):
    $SECRET_KEY

  First visitor to register becomes the instance admin. Send them the link.
DONE
    exit 0
  fi
  sleep 10
done
echo "Instance did not become healthy in 15 minutes. Investigate with:" >&2
echo "  ssh -p 2299 root@$IPV4 'cloud-init status --long; docker compose -f /etc/gitgit/docker-compose.yml logs'" >&2
exit 1

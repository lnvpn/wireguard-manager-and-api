#!/usr/bin/env bash
# Usage: scripts/upgrade-server.sh root@host [tag]
# Switches a server's /opt/wgManagerAPI compose stack to ghcr.io/lnvpn/wireguard-manager-and-api:<tag>.
# Config and database are backed up to /root first; if the new container fails to start, the
# previous compose file is restored and started again.
set -euo pipefail

HOST="${1:?usage: $0 root@host [tag]}"
TAG="${2:-v0.7.0}"
IMAGE="ghcr.io/lnvpn/wireguard-manager-and-api:${TAG}"

ssh -o BatchMode=yes "$HOST" IMAGE="$IMAGE" bash -s <<'EOF'
set -euo pipefail
cd /opt/wgManagerAPI
if command -v docker-compose >/dev/null; then DC="docker-compose"; else DC="docker compose"; fi

docker pull -q "$IMAGE"

TS=$(date -u +%Y%m%d-%H%M%S)
cp docker-compose.yml "/root/docker-compose.yml.$TS"
$DC down
tar czf "/root/wgManagerAPI-backup-$TS.tgz" --exclude=wgManagerAPI/logs -C /opt wgManagerAPI
sed -i "s#^\(\s*\)image:.*wireguard-manager-and-api.*#\1image: $IMAGE#" docker-compose.yml
grep -q "image: $IMAGE" docker-compose.yml

if ! $DC up -d; then
  cp "/root/docker-compose.yml.$TS" docker-compose.yml
  $DC up -d
  echo "ROLLED BACK: new image failed to start" >&2
  exit 1
fi

sleep 10
C=$($DC ps -q | head -1)
docker ps --filter "id=$C" --format "{{.Names}} {{.Status}} {{.Image}}"
echo "peers on wg0: $(docker exec "$C" wg show wg0 peers | wc -l)"
echo "backup: /root/wgManagerAPI-backup-$TS.tgz"
echo "rollback: cd /opt/wgManagerAPI && cp /root/docker-compose.yml.$TS docker-compose.yml && $DC down && $DC up -d"
EOF

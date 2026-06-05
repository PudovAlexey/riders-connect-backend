#!/usr/bin/env bash
# One-shot deploy for a fresh Ubuntu VPS, runnable straight from the web console:
#   curl -fsSL https://raw.githubusercontent.com/PudovAlexey/riders-connect-backend/main/deploy/up.sh | bash
#
# Installs Docker if missing, clones THIS (public) repo, generates secrets on the
# first run, and brings up Postgres + backend + nginx-served frontend. Re-running
# pulls the latest and rebuilds. Secrets persist in deploy/.env across runs.
set -euo pipefail

REPO_URL=https://github.com/PudovAlexey/riders-connect-backend.git
REPO_DIR=/opt/riders
DOMAIN="${DOMAIN:-motocade.ru}"   # public domain served over HTTPS by Caddy

echo ">>> [1/4] base packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq && apt-get install -y -qq git curl openssl ca-certificates >/dev/null

echo ">>> [2/4] docker"
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi
# Docker Hub anonymous pulls are rate-limited (and often blocked in RU). Probe a
# few public mirrors, keep the ones reachable from THIS server, and point Docker
# at them so image pulls/builds don't hit the limit.
if ! grep -q registry-mirrors /etc/docker/daemon.json 2>/dev/null; then
  : > /tmp/_mirrors
  for u in https://mirror.gcr.io https://huecker.io https://noohub.ru https://dockerhub.timeweb.cloud; do
    curl -s -o /dev/null --max-time 8 "$u/v2/" && echo "$u" >> /tmp/_mirrors && echo "    mirror reachable: $u"
  done
  if [ -s /tmp/_mirrors ]; then
    LIST=$(paste -sd, /tmp/_mirrors | sed 's/[^,]*/"&"/g')
    mkdir -p /etc/docker
    printf '{ "registry-mirrors": [%s] }\n' "$LIST" > /etc/docker/daemon.json
    systemctl restart docker || service docker restart || true
    sleep 3
  fi
fi

echo ">>> [3/4] code"
if [ -d "$REPO_DIR/.git" ]; then
  git -C "$REPO_DIR" pull --ff-only
else
  rm -rf "$REPO_DIR"
  git clone --depth 1 "$REPO_URL" "$REPO_DIR"
fi

ENV_FILE="$REPO_DIR/deploy/.env"
if [ ! -f "$ENV_FILE" ]; then
  cat > "$ENV_FILE" <<EOF
POSTGRES_PASSWORD=$(openssl rand -hex 16)
JWT_SECRET=$(openssl rand -hex 32)
PUBLIC_URL=https://$DOMAIN
SMTP_HOST=smtp.yandex.ru
SMTP_PORT=587
SMTP_FROM=pudo-aleksej@yandex.ru
SMTP_PASS=eodgnjsbffcnhqkt
EOF
  echo ">>> generated $ENV_FILE"
fi
# Keep media URLs on the HTTPS domain across re-runs (fixes old http://<ip> values
# without regenerating secrets, which would break the existing DB/JWT).
sed -i "s#^PUBLIC_URL=.*#PUBLIC_URL=https://$DOMAIN#" "$ENV_FILE"

echo ">>> [4/4] up"
cd "$REPO_DIR/deploy"
docker compose --env-file "$ENV_FILE" -f docker-compose.deploy.yml up -d --build

echo
echo ">>> DONE. Open: $(grep '^PUBLIC_URL=' "$ENV_FILE" | cut -d= -f2-)"
docker compose --env-file "$ENV_FILE" -f docker-compose.deploy.yml ps

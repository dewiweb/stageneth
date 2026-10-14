#!/bin/sh
# StageNeth HTTPS end-to-end test — the highest-fidelity check available
# without OpenWrt hardware.
#
#   docker nginx:alpine ──TLS──▶ unix socket ──▶ real stageneth-api
#
# Uses the packages' REAL *.locations files (SSI, CSP, HTTP→HTTPS
# redirect, API-over-HTTP refusal, proxy_buffering off for SSE) so the
# nginx layer is tested as shipped, not approximated.
#
#   tests/e2e-https/run.sh        # run and clean up
#   KEEP=1 tests/e2e-https/run.sh # leave env running (prints paths)
#
# Requires: docker, go, curl, openssl. No root needed.
set -u

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GO_DIR="$ROOT/packages/stagenethd/files"
WORK="$(mktemp -d)"
HTTP_PORT="${E2E_HTTP_PORT:-18080}"
HTTPS_PORT="${E2E_HTTPS_PORT:-18443}"
CONTAINER="stageneth-nginx-e2e"
PASS=0; FAIL=0

cleanup() {
	[ "${KEEP:-0}" = "1" ] && return
	docker rm -f "$CONTAINER" >/dev/null 2>&1
	[ -n "${API_PID:-}" ] && kill "$API_PID" 2>/dev/null
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

say() { printf '%s\n' "$*"; }
ok()  { PASS=$((PASS+1)); say "PASS $1"; }
bad() { FAIL=$((FAIL+1)); say "FAIL $1"; }

for t in docker go curl openssl; do
	command -v "$t" >/dev/null || { say "missing tool: $t"; exit 1; }
done

# --- build real binaries ----------------------------------------------------
( cd "$GO_DIR" && CGO_ENABLED=0 go build -mod=vendor -o "$WORK/stageneth-api" ./cmd/stageneth-api ) \
	|| { say "build stageneth-api failed"; exit 1; }
( cd "$GO_DIR" && CGO_ENABLED=0 go build -mod=vendor -o "$WORK/stagenethd" ./cmd/stagenethd ) \
	|| { say "build stagenethd failed"; exit 1; }

# --- seed environment --------------------------------------------------------
mkdir -p "$WORK/config" "$WORK/state" "$WORK/init.d" "$WORK/log" "$WORK/sock" "$WORK/tls" "$WORK/nginx-conf.d"
cp "$GO_DIR/config" "$WORK/config/stageneth"
cp "$GO_DIR/config.api" "$WORK/config/stagenethd"

for s in network firewall dnsmasq umdns mdns-repeater rsyslog stageneth-ptp system; do
	printf '#!/bin/sh\necho "$0 $*" >> "%s/initd.log"\nexit 0\n' "$WORK" > "$WORK/init.d/$s"
	chmod +x "$WORK/init.d/$s"
done

openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
	-subj '/CN=stageneth-e2e' -keyout "$WORK/tls/key.pem" -out "$WORK/tls/cert.pem" >/dev/null 2>&1

# nginx.conf mirrors OpenWrt's _lan/_ssl blocks: every server includes the
# same *.locations drop-ins.
cat > "$WORK/nginx.conf" <<NGINX
user root;
worker_processes 1;
pid /var/run/nginx.pid;
error_log /dev/stderr warn;
events { worker_connections 128; }
http {
    access_log off;
    server {
        listen 8080;
        server_name _;
        include /etc/nginx/conf.d/*.locations;
    }
    server {
        listen 8443 ssl;
        server_name _;
        ssl_certificate     /tls/cert.pem;
        ssl_certificate_key /tls/key.pem;
        include /etc/nginx/conf.d/*.locations;
    }
}
NGINX

# the REAL shipped locations files — this is the fidelity point of the test
cp "$GO_DIR/stageneth-api.locations" "$WORK/nginx-conf.d/"
cp "$ROOT/packages/stageneth-ui/files/stageneth-ui.locations" "$WORK/nginx-conf.d/"
# UI document root
cp -r "$ROOT/packages/stageneth-ui/files" "$WORK/www"
rm -f "$WORK/www/stageneth-ui.locations"
# socket path inside the container must match the locations file
sed -i 's|unix:/var/run/stageneth-api.sock|unix:/sock/stageneth-api.sock|' "$WORK/nginx-conf.d/stageneth-api.locations"

# --- run ---------------------------------------------------------------------
docker rm -f "$CONTAINER" >/dev/null 2>&1
docker run -d --name "$CONTAINER" \
	-p "$HTTP_PORT:8080" -p "$HTTPS_PORT:8443" \
	-v "$WORK/nginx.conf:/etc/nginx/nginx.conf:ro" \
	-v "$WORK/nginx-conf.d:/etc/nginx/conf.d:ro" \
	-v "$WORK/www:/www-stageneth:ro" \
	-v "$WORK/tls:/tls:ro" \
	-v "$WORK/sock:/sock" \
	nginx:alpine >/dev/null || { say "docker run failed"; exit 1; }

STAGENETH_CONFIG_DIR="$WORK/config" \
STAGENETH_STATE_DIR="$WORK/state" \
STAGENETH_INIT_DIR="$WORK/init.d" \
STAGENETHD_BIN="$WORK/stagenethd" \
STAGENETH_AUDIT_LOG="$WORK/log/audit.log" \
STAGENETH_SOCKET="$WORK/sock/stageneth-api.sock" \
	"$WORK/stageneth-api" >"$WORK/log/api.log" 2>&1 &
API_PID=$!

i=0
while [ ! -S "$WORK/sock/stageneth-api.sock" ]; do
	i=$((i+1)); [ $i -gt 50 ] && { say "API socket never appeared"; cat "$WORK/log/api.log"; exit 1; }
	sleep 0.1
done
sleep 0.5 # let nginx see the socket

BASE="https://localhost:$HTTPS_PORT"
PLAIN="http://localhost:$HTTP_PORT"
SESS=""
hdrs() { sed -n 's/^set-cookie: stageneth_sess=\([^;]*\).*/\1/ip' "$1" | tr -d '\r'; }

# --- assertions ---------------------------------------------------------------
c=$(curl -sk -o /dev/null -w '%{http_code}' "$BASE/api/v1/health")
[ "$c" = "200" ] && ok "https api health" || bad "https api health ($c)"

c=$(curl -s -o /dev/null -w '%{http_code}' "$PLAIN/api/v1/health")
[ "$c" = "403" ] && ok "http api refused" || bad "http api refused ($c)"

c=$(curl -s -o /dev/null -w '%{http_code}' "$PLAIN/")
[ "$c" = "301" ] && ok "http ui redirects" || bad "http ui redirects ($c)"

out=$(curl -sk -D "$WORK/h" -o /dev/null "$BASE/")
c=$(head -1 "$WORK/h" | grep -c 200)
[ "$c" = "1" ] && ok "https ui serves" || bad "https ui serves"
grep -qi 'content-security-policy' "$WORK/h" && ok "CSP header present" || bad "CSP header present"

# firstboot wizard provisions admin and returns a Secure session cookie
c=$(curl -sk -D "$WORK/h" -o "$WORK/b" -w '%{http_code}' -X POST "$BASE/api/v1/wizard" \
	-H 'Content-Type: application/json' \
	-d '{"username":"admin","password":"e2e-test-password","preset":"base"}')
[ "$c" = "200" ] && ok "wizard provisions" || bad "wizard provisions ($c: $(cat "$WORK/b"))"
SESS=$(hdrs "$WORK/h")
[ -n "$SESS" ] && ok "session cookie issued" || bad "session cookie issued"
grep -qi 'secure' "$WORK/h" && ok "cookie Secure through nginx" || bad "cookie Secure"

c=$(curl -sk -o /dev/null -w '%{http_code}' -H "Cookie: stageneth_sess=$SESS" "$BASE/api/v1/state")
[ "$c" = "200" ] && ok "authed state read" || bad "authed state read ($c)"

# SSE regression: the stream must push a data frame within ~7s through
# nginx proxy_buffering off. (A float64-only accessor once left this dead.)
sse=$(timeout 8 curl -skN -H "Cookie: stageneth_sess=$SESS" "$BASE/api/v1/events" | head -c 200000)
echo "$sse" | grep -q 'data: {' && ok "SSE frame pushed" || bad "SSE frame pushed"
echo "$sse" | grep -q '"collected_at"' && ok "SSE carries snapshot" || bad "SSE carries snapshot"

c=$(curl -sk -o "$WORK/b" -w '%{http_code}' -H "Cookie: stageneth_sess=$SESS" "$BASE/api/v1/metrics")
[ "$c" = "200" ] && grep -q 'stageneth_connections_max [1-9]' "$WORK/b" \
	&& ok "metrics real values" || bad "metrics real values ($c)"

# role guard: creating an operator then checking a write is refused
c=$(curl -sk -o /dev/null -w '%{http_code}' -H "Cookie: stageneth_sess=$SESS" \
	-H 'Content-Type: application/json' -X POST "$BASE/api/v1/users" \
	-d '{"username":"viewer","role":"operator","password":"viewer-pass-7"}')
[ "$c" = "200" ] && ok "operator created" || bad "operator created ($c)"
OID=$(curl -sk -D "$WORK/h" -o /dev/null -X POST "$BASE/api/v1/session" \
	-H 'Content-Type: application/json' -d '{"username":"viewer","password":"viewer-pass-7"}' >/dev/null; hdrs "$WORK/h")
c=$(curl -sk -o /dev/null -w '%{http_code}' -H "Cookie: stageneth_sess=$OID" \
	-H 'Content-Type: application/json' -X PUT "$BASE/api/v1/services/evil" \
	-d '{"vlan_id":"99","ipaddr":"10.99.0.1"}')
[ "$c" = "403" ] && ok "operator write refused" || bad "operator write refused ($c)"

# logout invalidates server-side
curl -sk -o /dev/null -X DELETE -H "Cookie: stageneth_sess=$SESS" "$BASE/api/v1/session"
c=$(curl -sk -o /dev/null -w '%{http_code}' -H "Cookie: stageneth_sess=$SESS" "$BASE/api/v1/state")
[ "$c" = "401" ] && ok "logout invalidates" || bad "logout invalidates ($c)"

say ""
say "$PASS passed, $FAIL failed"
if [ "${KEEP:-0}" = "1" ]; then
	say "env kept: WORK=$WORK  https://localhost:$HTTPS_PORT  (container: $CONTAINER)"
	trap - EXIT INT TERM
fi
[ "$FAIL" -eq 0 ]

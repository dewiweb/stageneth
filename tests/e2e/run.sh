#!/bin/sh
# StageNeth end-to-end smoke test.
#
# Runs the REAL stageneth-api + stagenethd binaries against an isolated
# config tree with stubbed init.d scripts — no OpenWrt required.
#
#   tests/e2e/run.sh
#
# Env knobs exercised:
#   STAGENETH_CONFIG_DIR  UCI root            (default /etc/config)
#   STAGENETH_STATE_DIR   manifest/last-good  (default /etc/stageneth)
#   STAGENETH_INIT_DIR    init.d remap        (default /etc/init.d)
#   STAGENETH_SOCKET      API unix socket
#   STAGENETHD_BIN        engine binary the API shells out to
#   STAGENETH_RSYSLOG_CONF rsyslog.conf path
set -u

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GO_DIR="$ROOT/packages/stagenethd/files"
WORK="$(mktemp -d)"
PASS=0; FAIL=0

cleanup() {
	[ -n "${API_PID:-}" ] && kill "$API_PID" 2>/dev/null
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

say() { printf '%s\n' "$*"; }
ok()   { PASS=$((PASS+1)); say "PASS $1"; }
bad()  { FAIL=$((FAIL+1)); say "FAIL $1"; }

# req METHOD PATH [JSON] [JAR] — prints "<http_code>", body in $BODY.
# Session cookies are Secure (correct: browser faces HTTPS via nginx), so
# curl's jar won't carry them over http — we capture Set-Cookie manually.
SESS=""
req() {
	m="$1"; p="$2"; d="${3:-}"
	args="--unix-socket $WORK/api.sock -s -o $WORK/resp.json -w %{http_code} -D $WORK/headers"
	[ -n "$SESS" ] && args="$args -H \"Cookie: stageneth_sess=$SESS\""
	if [ -n "$d" ]; then
		CODE=$(eval curl $args -X "$m" -H "'Content-Type: application/json'" -d "'$d'" "http://x$p")
	else
		CODE=$(eval curl $args -X "$m" "http://x$p")
	fi
	BODY=$(cat "$WORK/resp.json")
	new=$(sed -n 's/^set-cookie: stageneth_sess=\([^;]*\).*/\1/ip' "$WORK/headers" | tr -d '\r')
	[ -n "$new" ] && SESS="$new"
}
expect() { # expect CODE PATH — checks last CODE matches wanted
	[ "$CODE" = "$1" ] && ok "$2 ($CODE)" || bad "$2 — got $CODE, want $1"
}

say "== build =="
( cd "$GO_DIR" && CGO_ENABLED=0 go build -mod=vendor -o "$WORK/stagenethd" ./cmd/stagenethd \
	&& CGO_ENABLED=0 go build -mod=vendor -o "$WORK/stageneth-api" ./cmd/stageneth-api ) \
	|| { say "build failed"; exit 1; }

say "== seed config tree =="
mkdir -p "$WORK/config" "$WORK/state" "$WORK/init.d"
cat > "$WORK/config/stageneth" <<'EOF'
config stageneth 'globals'
	option enabled '1'
	option schema_version '2'
	option switch_vendor 'generic'
	option trunk 'eth1'
	option firstboot_done '0'

config service 'mgmt'
	option vlan_id '1'
	option ipaddr '192.168.10.2'
	option netmask '255.255.255.0'
EOF
cat > "$WORK/config/stagenethd" <<'EOF'
config main 'main'
	option enabled '1'
EOF
# stub init.d scripts — record invocations, always succeed; report
# 'active' for status probes so the service monitor sees them up
for s in network firewall dnsmasq rsyslog umdns mdns-repeater stageneth-ptp system; do
	cat > "$WORK/init.d/$s" <<EOF
#!/bin/sh
echo "$s \$*" >> "$WORK/init.log"
[ "\$1" = "status" ] && echo active
exit 0
EOF
	chmod +x "$WORK/init.d/$s"
done
touch "$WORK/rsyslog.conf"

say "== start API =="
STAGENETH_CONFIG_DIR="$WORK/config" \
STAGENETH_STATE_DIR="$WORK/state" \
STAGENETH_INIT_DIR="$WORK/init.d" \
STAGENETH_RSYSLOG_CONF="$WORK/rsyslog.conf" \
STAGENETH_SOCKET="$WORK/api.sock" \
STAGENETHD_BIN="$WORK/stagenethd" \
	"$WORK/stageneth-api" > "$WORK/api.log" 2>&1 &
API_PID=$!
for i in 1 2 3 4 5 6 7 8 9 10; do [ -S "$WORK/api.sock" ] && break; sleep 0.3; done
[ -S "$WORK/api.sock" ] || { say "API did not start:"; cat "$WORK/api.log"; exit 1; }

say "== firstboot flow =="
req GET /api/v1/firstboot
expect 200 "firstboot status"
echo "$BODY" | grep -q '"firstboot_done":false' && ok "firstboot not done" || bad "firstboot flag: $BODY"

req POST /api/v1/session '{"username":"admin","password":"wrong-password"}'
expect 401 "bad login rejected"

req POST /api/v1/wizard-skip '{"password":"e2e-secret-pass"}'
expect 200 "wizard provisions admin"
[ -n "$SESS" ] && ok "session cookie set" || bad "no session cookie"

req GET /api/v1/session
expect 200 "session check"
echo "$BODY" | grep -q '"role":"tech"' && ok "tech role" || bad "role: $BODY"

say "== typed v1 resources =="
req GET /api/v1/state
expect 200 "state"
echo "$BODY" | grep -q '"trunk":"eth1"' && ok "state has globals" || bad "state: $BODY"

req PUT /api/v1/services/dante \
	'{"vlan_id":"20","dscp":"46","priority":"4","mtu":"9000","ipaddr":"10.20.0.2","netmask":"255.255.255.0","ptp":"1","multicast":"1","untagged":"0"}'
expect 200 "service upsert"
grep -q "config service 'dante'" "$WORK/config/stageneth" && ok "service persisted" || bad "service not in file"

req PUT /api/v1/services/bad '{"vlan_id":"9999","ipaddr":"1.2.3.4","netmask":"255.255.255.0"}'
expect 400 "invalid vlan rejected"

req DELETE /api/v1/services/mgmt
expect 400 "mgmt delete refused"

req PUT /api/v1/bindings/b1 '{"interface":"eth1","service":"dante"}'
expect 200 "binding upsert"

req PUT /api/v1/globals '{"show_lock":"0","trunk":"eth1"}'
expect 200 "globals update"
grep -q "option trunk 'eth1'" "$WORK/config/stageneth" && ok "trunk preserved" || bad "trunk lost"

say "== engine apply via API =="
req POST /api/v1/apply '{}'
expect 200 "apply"
grep -q "config interface 'svc_dante'" "$WORK/config/network" && ok "network rendered" || { bad "network not rendered"; cat "$WORK/config/network"; }
grep -q 'network reload' "$WORK/init.log" && ok "network reload invoked" || bad "no reload: $(cat "$WORK/init.log" 2>/dev/null)"
[ -f "$WORK/state/manifest.json" ] && ok "manifest written" || bad "no manifest"
[ -d "$WORK/state/last-good" ] && ok "last-good snapshot saved" || bad "no last-good"

say "== show lock blocks apply =="
req PUT /api/v1/globals '{"show_lock":"1"}'
req POST /api/v1/apply '{}'
expect 409 "show lock → 409"
req PUT /api/v1/globals '{"show_lock":"0"}'

say "== preview =="
req GET /api/v1/apply/preview
expect 200 "preview"

say "== operator cannot write =="
req POST /api/v1/users '{"username":"stageop","role":"operator","password":"e2e-op-pass-1"}'
expect 200 "create operator"
OPSESS=$(curl --unix-socket "$WORK/api.sock" -s -D - -o /dev/null -X POST \
	-H 'Content-Type: application/json' \
	-d '{"username":"stageop","password":"e2e-op-pass-1"}' http://x/api/v1/session \
	| sed -n 's/^set-cookie: stageneth_sess=\([^;]*\).*/\1/ip' | tr -d '\r')
CODE=$(curl --unix-socket "$WORK/api.sock" -s -o /dev/null -w '%{http_code}' \
	-H "Cookie: stageneth_sess=$OPSESS" \
	-X PUT -H 'Content-Type: application/json' \
	-d '{"vlan_id":"30","ipaddr":"1.2.3.4","netmask":"255.255.255.0"}' http://x/api/v1/services/evil)
expect 403 "operator write denied"
CODE=$(curl --unix-socket "$WORK/api.sock" -s -o /dev/null -w '%{http_code}' \
	-H "Cookie: stageneth_sess=$OPSESS" http://x/api/v1/monitoring)
expect 200 "operator can read monitoring"

say "== console rollback =="
SDENV="STAGENETH_CONFIG_DIR=$WORK/config STAGENETH_STATE_DIR=$WORK/state STAGENETH_INIT_DIR=$WORK/init.d STAGENETH_RSYSLOG_CONF=$WORK/rsyslog.conf"
# last-good was saved before the first apply — no managed files existed
# then, so rollback must REMOVE the apply-created files entirely.
env $SDENV "$WORK/stagenethd" rollback > "$WORK/rollback.log" 2>&1 \
	&& ok "rollback ran" || { bad "rollback failed"; cat "$WORK/rollback.log"; }
[ ! -f "$WORK/config/network" ] && ok "rollback removed apply-created files" \
	|| bad "rollback left generated files: $(ls "$WORK/config")"

say "== logout revokes access =="
req DELETE /api/v1/session
expect 200 "logout"
req GET /api/v1/state
expect 401 "post-logout state denied"

say ""
say "== $PASS passed, $FAIL failed =="
[ "$FAIL" = 0 ]

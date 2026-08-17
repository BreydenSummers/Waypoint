#!/usr/bin/env bash
set -euo pipefail
umask 077

SCRIPT_NAME=$(basename "$0")
MODE=""
CONFIG_FILE=""
PROVISION_FILE=""
DESTROY_RECEIPT_FILE=""
DESTROY_BUNDLE_PATH=""
INSTALL_ROOT=""
STATE_ROOT=""
LOG_ROOT=""
DRY_RUN=0
DESTROY_FORCE=0
ROLLBACK_VERSION=""

FAIL_AT=${WAYPOINT_INSTALLER_FAIL_AT:-}
OS_RELEASE_FILE=${WAYPOINT_INSTALLER_OS_RELEASE_FILE:-/etc/os-release}
UNAME_MACHINE=${WAYPOINT_INSTALLER_UNAME_MACHINE:-$(uname -m)}

usage() {
  cat <<'EOF'
Usage: waypoint-installer.sh <validate|install|upgrade|provision|diagnostics|rollback|destroy> [options]
       waypoint-installer.sh rollback [version] [options]

Options:
  --config FILE       installer env file
  --provision FILE    account provisioning JSON file
  --receipt FILE      verified export receipt for destroy
  --bundle PATH       exact verified bundle path for destroy
  --force             break-glass teardown without a verified receipt
  --root DIR          install root (default from config or /opt/waypoint)
  --state-root DIR    state root (default from config or /var/lib/waypoint)
  --log-root DIR      log root (default from config or /var/log/waypoint)
  --dry-run           validate and render, but do not mutate
EOF
}

log() {
  printf '%s: %s\n' "$SCRIPT_NAME" "$*" >&2
}

die() {
  local msg=$1
  record_failure "$msg"
  log "$msg"
  exit 1
}

trim() {
  local s=$1
  s=${s#${s%%[![:space:]]*}}
  s=${s%${s##*[![:space:]]}}
  printf '%s' "$s"
}

sql_quote() {
  local s=$1
  s=${s//\'/\'\'}
  printf "'%s'" "$s"
}

sha256_file() {
  sha256sum "$1" | awk '{print $1}'
}

read_env_file() {
  local file=$1
  [[ -f $file ]] || die "missing config file: $file"
  while IFS= read -r line || [[ -n $line ]]; do
    line=${line%$'\r'}
    [[ -z ${line//[[:space:]]/} ]] && continue
    [[ ${line:0:1} == '#' ]] && continue
    case $line in
      *=*) ;;
      *) die "invalid config line (expected KEY=VALUE): $line" ;;
    esac
    local key=${line%%=*}
    local value=${line#*=}
    key=$(trim "$key")
    value=$(trim "$value")
    if [[ ${value:0:1} == '"' && ${value: -1} == '"' ]]; then
      value=${value:1:${#value}-2}
    elif [[ ${value:0:1} == "'" && ${value: -1} == "'" ]]; then
      value=${value:1:${#value}-2}
    fi
    CONFIG["$key"]=$value
  done < "$file"
}

cfg() {
  local key=$1
  printf '%s' "${CONFIG[$key]:-}"
}

require_cfg() {
  local key=$1
  local value
  value=$(cfg "$key")
  [[ -n $value ]] || die "missing required config: $key"
}

read_os_field() {
  local key=$1
  local value
  value=$(awk -F= -v key="$key" '$1==key {gsub(/^"|"$/, "", $2); print $2; exit}' "$OS_RELEASE_FILE" 2>/dev/null || true)
  printf '%s' "$value"
}

db_host_from_dsn() {
  local dsn=$1
  python3 - "$dsn" <<'PY'
from urllib.parse import urlparse
import sys

dsn = sys.argv[1].strip()
host = ""
if dsn:
    parsed = urlparse(dsn)
    host = parsed.hostname or ""
print(host)
PY
}

db_dsn_components() {
  local dsn=$1
  python3 - "$dsn" <<'PY'
from urllib.parse import urlparse
import sys

dsn = sys.argv[1].strip()
parsed = urlparse(dsn) if dsn else None
host = parsed.hostname if parsed and parsed.hostname else ""
port = str(parsed.port) if parsed and parsed.port else ""
dbname = (parsed.path or "").lstrip("/") if parsed else ""
user = parsed.username if parsed and parsed.username else ""
password = parsed.password if parsed and parsed.password else ""
print(host)
print(port)
print(dbname)
print(user)
print(password)
PY
}

postgres_superuser_psql() {
  if command -v sudo >/dev/null 2>&1; then
    sudo -n -u postgres -- psql -d postgres -X -v ON_ERROR_STOP=1 "$@"
    return $?
  fi
  if command -v runuser >/dev/null 2>&1; then
    runuser -u postgres -- psql -d postgres -X -v ON_ERROR_STOP=1 "$@"
    return $?
  fi
  psql -d postgres -X -v ON_ERROR_STOP=1 "$@"
}

bootstrap_local_database() {
  local dsn dbname user password
  local -a dsn_parts=()
  dsn=$(cfg WAYPOINT_DB_DSN)
  mapfile -t dsn_parts < <(db_dsn_components "$dsn")
  dbname=${dsn_parts[2]:-}
  user=${dsn_parts[3]:-}
  password=${dsn_parts[4]:-}
  [[ -n $dbname && -n $user ]] || return 0

  if ! postgres_superuser_psql -tAc "SELECT 1 FROM pg_roles WHERE rolname = :'role_name'" -v role_name="$user" | grep -qx 1; then
    postgres_superuser_psql -v role_name="$user" -v role_password="$password" -c "CREATE ROLE :\"role_name\" LOGIN PASSWORD :'role_password'"
  else
    postgres_superuser_psql -v role_name="$user" -v role_password="$password" -c "ALTER ROLE :\"role_name\" LOGIN PASSWORD :'role_password'"
  fi

  if ! postgres_superuser_psql -tAc "SELECT 1 FROM pg_database WHERE datname = :'db_name'" -v db_name="$dbname" | grep -qx 1; then
    postgres_superuser_psql -v db_name="$dbname" -v db_owner="$user" -c "CREATE DATABASE :\"db_name\" OWNER :\"db_owner\""
  else
    postgres_superuser_psql -v db_name="$dbname" -v db_owner="$user" -c "ALTER DATABASE :\"db_name\" OWNER TO :\"db_owner\""
  fi
}

is_local_database() {
  case $(db_host_from_dsn "$(cfg WAYPOINT_DB_DSN)") in
    ""|localhost|127.0.0.1|::1) return 0 ;;
    *) return 1 ;;
  esac
}

wait_for_waypoint_ready() {
  local url
  url="$(cfg WAYPOINT_PUBLIC_URL)"
  url="${url%/}/readyz"
  local attempt
  for attempt in $(seq 1 30); do
    if python3 - "$url" <<'PY' >/dev/null 2>&1
from urllib.request import urlopen
import sys

url = sys.argv[1]
with urlopen(url, timeout=5) as response:
    response.read()
PY
    then
      return 0
    fi
    sleep 1
  done
  die "waypoint did not become ready at $url"
}

sync_waypoint_service() {
  systemctl daemon-reload
  systemctl enable waypoint
  systemctl restart waypoint
  wait_for_waypoint_ready
}

ensure_local_postgres() {
  is_local_database || return 0
  if ! command -v psql >/dev/null 2>&1 || ! command -v pg_isready >/dev/null 2>&1; then
    command -v apt-get >/dev/null 2>&1 || die "apt-get required to install PostgreSQL tooling"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y postgresql postgresql-client
  fi
  systemctl enable --now postgresql
  local attempt
  for attempt in $(seq 1 30); do
    if pg_isready -d postgres >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  if ! pg_isready -d postgres >/dev/null 2>&1; then
    die "postgresql did not become ready"
  fi
  bootstrap_local_database
  for attempt in $(seq 1 30); do
    if pg_isready -d "$(cfg WAYPOINT_DB_DSN)" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  die "postgresql did not become ready"
}

validate_host() {
  local os_id os_version
  os_id=$(read_os_field ID)
  os_version=$(read_os_field VERSION_ID)
  [[ $UNAME_MACHINE == x86_64 ]] || die "unsupported architecture: $UNAME_MACHINE (need x86_64)"
  [[ $os_id == ubuntu ]] || die "unsupported OS: ${os_id:-unknown} (need Ubuntu 22.04 or 24.04)"
  case $os_version in
    22.04|24.04) ;;
    *) die "unsupported Ubuntu version: ${os_version:-unknown} (need 22.04 or 24.04)" ;;
  esac
}

validate_version() {
  local version=$1
  [[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9]+)*$ ]] || die "invalid WAYPOINT_VERSION: $version"
}

validate_package_path() {
  local package_path=$1
  [[ -e $package_path ]] || die "WAYPOINT_PACKAGE_PATH does not exist: $package_path"
  [[ -r $package_path ]] || die "WAYPOINT_PACKAGE_PATH is not readable: $package_path"
}

validate_provision_file() {
  [[ -n ${PROVISION_FILE:-} ]] || return 0
  [[ -f $PROVISION_FILE ]] || die "missing provision file: $PROVISION_FILE"
  python3 - "$PROVISION_FILE" <<'PY'
import json, re, sys
path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as fh:
    data = json.load(fh)
if not isinstance(data, dict):
    raise SystemExit('provision file must be a JSON object')
eng = data.get('engagement')
actors = data.get('actors')
if not isinstance(eng, dict):
    raise SystemExit('provision.engagement must be an object')
for key in ('id', 'name', 'client', 'scope'):
    if not str(eng.get(key, '')).strip():
        raise SystemExit(f'missing engagement.{key}')
if not isinstance(actors, list) or not actors:
    raise SystemExit('provision.actors must be a non-empty array')
uuid_re = re.compile(r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$')
seen = set()
human_ids = set()
for actor in actors:
    if not isinstance(actor, dict):
        raise SystemExit('each actor must be an object')
    for key in ('id', 'kind', 'handle', 'role'):
        if not str(actor.get(key, '')).strip():
            raise SystemExit(f'missing actor.{key}')
    if not uuid_re.match(actor['id']):
        raise SystemExit(f'invalid actor.id: {actor["id"]}')
    if actor['id'] in seen:
        raise SystemExit(f'duplicate actor.id: {actor["id"]}')
    seen.add(actor['id'])
    if actor['kind'] not in ('human', 'ai_agent'):
        raise SystemExit(f'invalid actor.kind: {actor["kind"]}')
    if actor['kind'] == 'human':
        human_ids.add(actor['id'])
        for key in ('agent_name', 'model', 'version', 'authorized_by'):
            if actor.get(key) not in (None, '', []):
                raise SystemExit(f'human actor must not set {key}')
    else:
        for key in ('agent_name', 'model', 'version', 'authorized_by'):
            if not str(actor.get(key, '')).strip():
                raise SystemExit(f'ai_agent actor missing {key}')
        if not uuid_re.match(actor['authorized_by']):
            raise SystemExit('invalid actor.authorized_by')
if not human_ids:
    raise SystemExit('provision requires at least one human actor')
for actor in actors:
    if actor['kind'] == 'ai_agent' and actor['authorized_by'] not in human_ids:
        raise SystemExit('ai_agent authorized_by must reference a human actor in the same provision file')
PY
}

emit_provision_sql() {
  local provision_file=$1
  local token_dir=$2
  python3 - "$provision_file" "$token_dir" <<'PY'
import hashlib, json, os, pathlib, secrets, stat, sys

def sql(s: str) -> str:
    return "'" + s.replace("'", "''") + "'"

path = pathlib.Path(sys.argv[1])
token_dir = pathlib.Path(sys.argv[2])
with path.open('r', encoding='utf-8') as fh:
    data = json.load(fh)
eng = data['engagement']
actors = data['actors']
token_dir.mkdir(parents=True, exist_ok=True)

print('BEGIN;')
print(
    'INSERT INTO engagement (id, name, client, scope, status) VALUES '
    f"({sql(eng['id'])}, {sql(eng['name'])}, {sql(eng['client'])}, {sql(eng['scope'])}, {sql(eng.get('status', 'active'))}) "
    'ON CONFLICT (id) DO UPDATE SET '
    'name = EXCLUDED.name, client = EXCLUDED.client, scope = EXCLUDED.scope, status = EXCLUDED.status, updated_at = now();'
)

for actor in actors:
    token_file = token_dir / f"{actor['id']}.token"
    if token_file.exists():
        token = token_file.read_text(encoding='utf-8').strip()
    else:
        token = secrets.token_urlsafe(32).rstrip('=')
        token_file.write_text(token + '\n', encoding='utf-8')
        os.chmod(token_file, stat.S_IRUSR | stat.S_IWUSR)
    token_hash = hashlib.sha256(token.encode('utf-8')).hexdigest()
    agent_name = actor.get('agent_name', '') if actor['kind'] == 'ai_agent' else ''
    model = actor.get('model', '') if actor['kind'] == 'ai_agent' else ''
    version = actor.get('version', '') if actor['kind'] == 'ai_agent' else ''
    authorized_by = actor.get('authorized_by', '') if actor['kind'] == 'ai_agent' else ''
    print(
        'INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES '
        f"({sql(actor['id'])}, {sql(eng['id'])}, {sql(actor['kind'])}, {sql(actor['handle'])}, {sql(token_hash)}, {sql(actor['role'])}, "
        f"{sql(agent_name) if agent_name else 'NULL'}, {sql(model) if model else 'NULL'}, {sql(version) if version else 'NULL'}, {sql(authorized_by) if authorized_by else 'NULL'}) "
        'ON CONFLICT (id) DO UPDATE SET '
        'engagement_id = EXCLUDED.engagement_id, kind = EXCLUDED.kind, handle = EXCLUDED.handle, token_hash = EXCLUDED.token_hash, '
        'role = EXCLUDED.role, agent_name = EXCLUDED.agent_name, model = EXCLUDED.model, version = EXCLUDED.version, '
        'authorized_by = EXCLUDED.authorized_by, revoked_at = NULL, updated_at = now();'
    )

print('COMMIT;')
PY
}

write_atomic() {
  local path=$1
  local content=$2
  local dir
  dir=$(dirname "$path")
  mkdir -p "$dir"
  local tmp
  tmp=$(mktemp "$dir/.tmp.XXXXXX")
  printf '%s' "$content" > "$tmp"
  mv "$tmp" "$path"
}

append_optional_runtime_config() {
  local path=$1
  local key value
  for key in WAYPOINT_TLS_CERT_FILE WAYPOINT_TLS_KEY_FILE WAYPOINT_TLS_CA_FILE WAYPOINT_EGRESS_MODE WAYPOINT_EGRESS_ENDPOINT WAYPOINT_EGRESS_ADDRESS; do
    value=$(cfg "$key")
    [[ -n $value ]] || continue
    printf '%s=%s\n' "$key" "$value" >> "$path"
  done
}

prepare_roots() {
  mkdir -p "$INSTALL_ROOT" "$STATE_ROOT" "$LOG_ROOT"
  mkdir -p "$INSTALL_ROOT/releases" "$STATE_ROOT/tokens" "$LOG_ROOT/installer"
}

current_release_path() {
  printf '%s' "$INSTALL_ROOT/current"
}

current_release_dir() {
  local link
  link=$(current_release_path)
  if [[ -L $link ]]; then
    readlink -f "$link"
  elif [[ -e $link ]]; then
    readlink -f "$link" 2>/dev/null || printf '%s' "$link"
  else
    printf ''
  fi
}

current_version() {
  local state_file=$STATE_ROOT/install.state
  [[ -f $state_file ]] || return 0
  awk -F= '$1=="WAYPOINT_INSTALLED_VERSION" {print $2; exit}' "$state_file"
}

record_failure() {
  local message=$1
  [[ -n ${LOG_ROOT:-} ]] || return 0
  mkdir -p "$LOG_ROOT/installer"
  {
    printf 'timestamp=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'mode=%s\n' "${MODE:-unknown}"
    printf 'message=%s\n' "$message"
    [[ -n ${FAIL_AT:-} ]] && printf 'fail_at=%s\n' "$FAIL_AT"
    [[ -n ${CONFIG_FILE:-} ]] && printf 'config=%s\n' "$CONFIG_FILE"
    [[ -n ${PROVISION_FILE:-} ]] && printf 'provision=%s\n' "$PROVISION_FILE"
    printf 'install_root=%s\n' "$INSTALL_ROOT"
    printf 'state_root=%s\n' "$STATE_ROOT"
  } > "$LOG_ROOT/installer/last-failure.txt"
  chmod 600 "$LOG_ROOT/installer/last-failure.txt" 2>/dev/null || true
}

restore_backup() {
  local backup_dir=$1
  local release_dir=$2
  local previous_link=$3
  if [[ -d $backup_dir/release ]]; then
    rm -rf "$release_dir"
    mkdir -p "$(dirname "$release_dir")"
    cp -a "$backup_dir/release" "$release_dir"
  else
    rm -rf "$release_dir"
  fi
  if [[ -n $previous_link ]]; then
    rm -f "$INSTALL_ROOT/current"
    ln -s "$previous_link" "$INSTALL_ROOT/current"
  else
    rm -f "$INSTALL_ROOT/current"
  fi
  if [[ -d $backup_dir/state ]]; then
    mkdir -p "$STATE_ROOT"
    if [[ -f $backup_dir/state/install.state ]]; then
      cp -a "$backup_dir/state/install.state" "$STATE_ROOT/install.state"
    else
      rm -f "$STATE_ROOT/install.state"
    fi
    if [[ -f $backup_dir/state/config.env ]]; then
      cp -a "$backup_dir/state/config.env" "$STATE_ROOT/config.env"
    else
      rm -f "$STATE_ROOT/config.env"
    fi
    if [[ -d $backup_dir/state/tokens ]]; then
      rm -rf "$STATE_ROOT/tokens"
      cp -a "$backup_dir/state/tokens" "$STATE_ROOT/tokens"
    else
      rm -rf "$STATE_ROOT/tokens"
    fi
  fi
}

fail_point() {
  local point=$1
  [[ -n $FAIL_AT && $FAIL_AT == "$point" ]]
}

install_or_upgrade() {
  local desired_version=$1
  local package_path=$2
  local release_dir="$INSTALL_ROOT/releases/$desired_version"
  local previous_link=""
  local backup_dir=""
  local state_file="$STATE_ROOT/install.state"
  local current_dir=""
  local current_ver=""

  current_dir=$(current_release_dir)
  current_ver=$(current_version || true)
  if [[ -n $current_ver && $current_ver == "$desired_version" && -L $(current_release_path) ]]; then
    if [[ $(cfg WAYPOINT_PACKAGE_PATH) == "$package_path" && $(sha256_file "$CONFIG_FILE") == $(awk -F= '$1=="WAYPOINT_CONFIG_SHA256" {print $2; exit}' "$state_file" 2>/dev/null || true) ]]; then
      log "already installed at version $desired_version"
      return 0
    fi
  fi

  if [[ -n $current_dir && -d $current_dir ]]; then
    previous_link=$current_dir
    backup_dir="$STATE_ROOT/rollback/$(date -u +%Y%m%d%H%M%S)"
    mkdir -p "$backup_dir/release" "$backup_dir/state"
    cp -a "$current_dir/." "$backup_dir/release/"
    [[ -f $STATE_ROOT/install.state ]] && cp -a "$STATE_ROOT/install.state" "$backup_dir/state/install.state"
    [[ -f $STATE_ROOT/config.env ]] && cp -a "$STATE_ROOT/config.env" "$backup_dir/state/config.env"
    [[ -d $STATE_ROOT/tokens ]] && cp -a "$STATE_ROOT/tokens" "$backup_dir/state/tokens"
  fi

  mkdir -p "$(dirname "$release_dir")"
  rm -rf "$release_dir"
  local temp_release
  temp_release=$(mktemp -d "$INSTALL_ROOT/releases/.staging.$desired_version.XXXXXX")
  mkdir -p "$temp_release/bin" "$temp_release/systemd"
  cp "$package_path" "$temp_release/bin/waypoint"
  chmod 0755 "$temp_release/bin/waypoint"

  cat > "$STATE_ROOT/config.env" <<EOF
WAYPOINT_VERSION=$(cfg WAYPOINT_VERSION)
WAYPOINT_PUBLIC_URL=$(cfg WAYPOINT_PUBLIC_URL)
WAYPOINT_DB_DSN=$(cfg WAYPOINT_DB_DSN)
WAYPOINT_PACKAGE_PATH=$package_path
EOF
  append_optional_runtime_config "$STATE_ROOT/config.env"
  chmod 600 "$STATE_ROOT/config.env"
  cat > "$temp_release/waypoint.env" <<EOF
WAYPOINT_VERSION=$(cfg WAYPOINT_VERSION)
WAYPOINT_PUBLIC_URL=$(cfg WAYPOINT_PUBLIC_URL)
WAYPOINT_DB_DSN=$(cfg WAYPOINT_DB_DSN)
EOF
  append_optional_runtime_config "$temp_release/waypoint.env"
  chmod 600 "$temp_release/waypoint.env"
  cat > "$temp_release/systemd/waypoint.service" <<EOF
[Unit]
Description=Waypoint
After=network-online.target

[Service]
Type=simple
EnvironmentFile=$STATE_ROOT/config.env
ExecStart=$INSTALL_ROOT/current/bin/waypoint
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

  mv "$temp_release" "$release_dir"
  ln -sfn "$release_dir" "$INSTALL_ROOT/current"

  ensure_local_postgres
  sync_waypoint_service

  if fail_point after_release; then
    restore_backup "$backup_dir" "$release_dir" "$previous_link"
    if [[ -n $previous_link ]]; then
      sync_waypoint_service
    else
      systemctl stop waypoint >/dev/null 2>&1 || true
    fi
    die "failure injected after release materialization"
  fi

  if [[ -n ${PROVISION_FILE:-} ]]; then
    provision_accounts
  fi

  if fail_point after_provision; then
    restore_backup "$backup_dir" "$release_dir" "$previous_link"
    if [[ -n $previous_link ]]; then
      sync_waypoint_service
    fi
    die "failure injected after provisioning"
  fi

  {
    printf 'WAYPOINT_INSTALLED_VERSION=%s\n' "$desired_version"
    printf 'WAYPOINT_CONFIG_SHA256=%s\n' "$(sha256_file "$CONFIG_FILE")"
    printf 'WAYPOINT_PACKAGE_SHA256=%s\n' "$(sha256_file "$package_path")"
    printf 'WAYPOINT_PACKAGE_PATH=%s\n' "$package_path"
    printf 'WAYPOINT_INSTALLED_AT=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } > "$state_file"
  chmod 600 "$state_file"

  rm -rf "$backup_dir"
}

provision_accounts() {
  [[ -n ${PROVISION_FILE:-} ]] || return 0
  validate_provision_file
  local token_dir="$STATE_ROOT/tokens"
  local sql_file
  sql_file=$(mktemp "$STATE_ROOT/.provision.XXXXXX.sql")
  emit_provision_sql "$PROVISION_FILE" "$token_dir" > "$sql_file"
  local db_dsn
  db_dsn=$(cfg WAYPOINT_DB_DSN)
  command -v psql >/dev/null 2>&1 || die "psql not available for provisioning"
  psql "$db_dsn" -X -v ON_ERROR_STOP=1 -f "$sql_file" >/dev/null
  mv "$sql_file" "$STATE_ROOT/last-provision.sql"
  chmod 600 "$STATE_ROOT/last-provision.sql"
}

diagnostics() {
  local state_file="$STATE_ROOT/install.state"
  echo "install_root=$INSTALL_ROOT"
  echo "state_root=$STATE_ROOT"
  echo "log_root=$LOG_ROOT"
  echo "current_release=$(current_release_dir)"
  echo "installed_version=$(current_version || true)"
  echo "waypoint_service=$(systemctl is-active waypoint 2>/dev/null || true)"
  if is_local_database; then
    echo "postgres_service=$(systemctl is-active postgresql 2>/dev/null || true)"
    echo "database_ready=$(pg_isready -d "$(cfg WAYPOINT_DB_DSN)" >/dev/null 2>&1 && echo ready || echo not-ready)"
  fi
  if [[ -f $state_file ]]; then
    cat "$state_file"
  fi
  if [[ -f $LOG_ROOT/installer/last-failure.txt ]]; then
    echo "--- last failure ---"
    cat "$LOG_ROOT/installer/last-failure.txt"
  fi
}

rollback() {
  local version=${1:-}
  local current_dir
  current_dir=$(current_release_dir)
  [[ -n $current_dir ]] || die "no current release to roll back"
  local backup_dir="$STATE_ROOT/rollback/$(date -u +%Y%m%d%H%M%S)"
  mkdir -p "$backup_dir"
  cp -a "$current_dir/." "$backup_dir/"
  if [[ -n $version ]]; then
    local target="$INSTALL_ROOT/releases/$version"
    local state_file="$STATE_ROOT/install.state"
    local tmp_state
    [[ -d $target ]] || die "rollback target not found: $target"
    if [[ -f $state_file ]]; then
      tmp_state=$(mktemp "$STATE_ROOT/.install.state.XXXXXX")
      awk -F= -v version="$version" 'BEGIN { OFS="=" }
        $1=="WAYPOINT_INSTALLED_VERSION" {$0="WAYPOINT_INSTALLED_VERSION=" version}
        {print}
      ' "$state_file" > "$tmp_state"
      mv "$tmp_state" "$state_file"
    else
      printf 'WAYPOINT_INSTALLED_VERSION=%s\n' "$version" > "$state_file"
    fi
    ln -sfn "$target" "$INSTALL_ROOT/current"
    sync_waypoint_service
  fi
  echo "$backup_dir"
}

verify_export_receipt() {
  local receipt_file=$1
  local bundle_path=$2
  python3 - "$receipt_file" "$bundle_path" <<'PY'
import json
import pathlib
import sys

receipt_path = pathlib.Path(sys.argv[1]).resolve(strict=False)
bundle_path = pathlib.Path(sys.argv[2]).resolve(strict=False)
with receipt_path.open('r', encoding='utf-8') as fh:
    data = json.load(fh)

if not isinstance(data, dict):
    raise SystemExit('receipt must be a JSON object')

status = str(data.get('status', '')).strip().lower()
verified = data.get('verified') is True or status in {'verified', 'hash-verified', 'sha-256 verified'}
if not verified:
    raise SystemExit('receipt is not verified')

receipt_bundle = data.get('bundlePath') or data.get('bundle_path') or data.get('verifiedBundlePath')
if not receipt_bundle and isinstance(data.get('bundle'), dict):
    receipt_bundle = data['bundle'].get('path')
if not receipt_bundle:
    raise SystemExit('receipt missing bundlePath')

if pathlib.Path(str(receipt_bundle)).resolve(strict=False) != bundle_path:
    raise SystemExit('receipt bundle path does not match the requested bundle')
PY
}

destroy() {
  [[ -n ${DESTROY_BUNDLE_PATH:-} ]] || die "--bundle is required for destroy"
  if [[ $DESTROY_FORCE -ne 1 ]]; then
    [[ -n ${DESTROY_RECEIPT_FILE:-} ]] || die "--receipt is required unless --force is used"
    [[ -f $DESTROY_RECEIPT_FILE ]] || die "missing receipt file: $DESTROY_RECEIPT_FILE"
    [[ -e $DESTROY_BUNDLE_PATH ]] || die "missing bundle path: $DESTROY_BUNDLE_PATH"
    verify_export_receipt "$DESTROY_RECEIPT_FILE" "$DESTROY_BUNDLE_PATH"
  else
    log "BREAK-GLASS: destroy proceeding without verified receipt"
  fi

  systemctl stop waypoint >/dev/null 2>&1 || true
  systemctl disable waypoint >/dev/null 2>&1 || true
  if is_local_database; then
    systemctl stop postgresql >/dev/null 2>&1 || true
  fi

  rm -rf "$INSTALL_ROOT" "$STATE_ROOT" "$LOG_ROOT"
  log "destroyed instance after bundle check: $DESTROY_BUNDLE_PATH"
}

load_config() {
  [[ -n ${CONFIG_FILE:-} ]] || die "--config is required"
  declare -gA CONFIG=()
  read_env_file "$CONFIG_FILE"
  require_cfg WAYPOINT_VERSION
  require_cfg WAYPOINT_PUBLIC_URL
  require_cfg WAYPOINT_DB_DSN
  require_cfg WAYPOINT_PACKAGE_PATH
  validate_version "$(cfg WAYPOINT_VERSION)"
  validate_package_path "$(cfg WAYPOINT_PACKAGE_PATH)"
  INSTALL_ROOT=${INSTALL_ROOT:-$(cfg WAYPOINT_INSTALL_ROOT)}
  STATE_ROOT=${STATE_ROOT:-$(cfg WAYPOINT_STATE_ROOT)}
  LOG_ROOT=${LOG_ROOT:-$(cfg WAYPOINT_LOG_ROOT)}
  INSTALL_ROOT=${INSTALL_ROOT:-/opt/waypoint}
  STATE_ROOT=${STATE_ROOT:-/var/lib/waypoint}
  LOG_ROOT=${LOG_ROOT:-/var/log/waypoint}
}

main() {
  while [[ $# -gt 0 ]]; do
    case $1 in
      --config) CONFIG_FILE=${2:-}; shift 2 ;;
      --provision) PROVISION_FILE=${2:-}; shift 2 ;;
      --receipt) DESTROY_RECEIPT_FILE=${2:-}; shift 2 ;;
      --bundle) DESTROY_BUNDLE_PATH=${2:-}; shift 2 ;;
      --root) INSTALL_ROOT=${2:-}; shift 2 ;;
      --state-root) STATE_ROOT=${2:-}; shift 2 ;;
      --log-root) LOG_ROOT=${2:-}; shift 2 ;;
      --dry-run) DRY_RUN=1; shift ;;
      --force) DESTROY_FORCE=1; shift ;;
      validate|install|upgrade|provision|diagnostics|rollback|destroy)
        MODE=$1
        shift
        ;;
      *)
        if [[ $MODE == rollback && -z $ROLLBACK_VERSION && ${1#-} == $1 ]]; then
          ROLLBACK_VERSION=$1
          shift
        else
          die "unknown argument: $1"
        fi
        ;;
      -h|--help)
        usage
        exit 0
        ;;
    esac
  done

  [[ -n $MODE ]] || { usage; exit 1; }
  load_config
  validate_host
  validate_provision_file

  case $MODE in
    validate)
      echo "config-ok"
      ;;
    install)
      prepare_roots
      [[ $DRY_RUN -eq 1 ]] || install_or_upgrade "$(cfg WAYPOINT_VERSION)" "$(cfg WAYPOINT_PACKAGE_PATH)"
      ;;
    upgrade)
      prepare_roots
      [[ $DRY_RUN -eq 1 ]] || install_or_upgrade "$(cfg WAYPOINT_VERSION)" "$(cfg WAYPOINT_PACKAGE_PATH)"
      ;;
    provision)
      prepare_roots
      [[ $DRY_RUN -eq 1 ]] || provision_accounts
      ;;
    diagnostics)
      diagnostics
      ;;
    rollback)
      prepare_roots
      rollback "$ROLLBACK_VERSION"
      ;;
    destroy)
      destroy
      ;;
  esac
}

main "$@"

#!/usr/bin/env bash

# Create the credential files that the batch-submit scripts need and that are
# missing, using the secrets in vault (https://vault.nbis.se).
#
# Currently: ~/.sda/s3cmd-bp-master-inbox.conf (s3cmd config of the BigPicture inbox).
# Existing files are never overwritten unless --force is given.
set -euo pipefail

SDA_DIR="$HOME/.sda"
S3CMD_INBOX_CONF="$SDA_DIR/s3cmd-bp-master-inbox.conf"

usage() {
    echo "Usage: $0 [--force] [-h]"
    echo "  Creates ${S3CMD_INBOX_CONF} from vault if it does not exist."
    echo "  --force  Replace the file even if it exists"
    echo "  -h       Show this help message"
    echo ""
    echo "  You must be logged in to vault (vault login ...)."
    exit 1
}

FORCE=0
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=1 ;;
        -h|--help) usage ;;
        *) echo "[-] Error: Unknown argument '$arg'." >&2; usage ;;
    esac
done

export VAULT_ADDR="${VAULT_ADDR:-https://vault.nbis.se}"

if [[ -f "$S3CMD_INBOX_CONF" && $FORCE -eq 0 ]]; then
    echo "[+] ${S3CMD_INBOX_CONF} exists, nothing to do."
    exit 0
fi

if ! command -v vault >/dev/null 2>&1; then
    echo "[-] Error: vault is not in PATH, cannot create ${S3CMD_INBOX_CONF}." >&2
    exit 1
fi
if ! vault token renew >/dev/null 2>&1; then
    echo "[-] Error: You must log in to ${VAULT_ADDR} before ${S3CMD_INBOX_CONF} can be created." >&2
    exit 1
fi

echo "[+] Creating ${S3CMD_INBOX_CONF} from vault..."

# Read everything first, so that a vault error does not leave a half-written file
ACCESS_KEY_INBOX=$(vault kv get -field=access_key bp-secrets/S3_keys/STO2/inbox)
SECRET_KEY_INBOX=$(vault kv get -field=secret_key bp-secrets/S3_keys/STO2/inbox)
# The S3 endpoint is the second entry of the comma-separated list; s3cmd wants no scheme
HOST=$(vault kv get -field=endpoints bp-secrets/S3_keys/STO2 | cut -d, -f2)
HOST="${HOST#http://}"
HOST="${HOST#https://}"
HOST="${HOST%/}"

if [[ -z "$ACCESS_KEY_INBOX" || -z "$SECRET_KEY_INBOX" || -z "$HOST" ]]; then
    echo "[-] Error: vault returned an empty access key, secret key or endpoint." >&2
    exit 1
fi

mkdir -p "$SDA_DIR"
umask 077
TMP_CONF=$(mktemp "${S3CMD_INBOX_CONF}.XXXXXX")
trap 'rm -f "$TMP_CONF"' EXIT

{
    echo "[default]"
    echo "access_key = ${ACCESS_KEY_INBOX}"
    echo "secret_key = ${SECRET_KEY_INBOX}"
    echo "host_base = ${HOST}"
    echo "host_bucket = ${HOST}"
    echo "use_https = True"
    echo "check_ssl_certificate = True"
    echo "signature_v2 = False"
} > "$TMP_CONF"

chmod 600 "$TMP_CONF"
mv "$TMP_CONF" "$S3CMD_INBOX_CONF"
echo "[+] Wrote ${S3CMD_INBOX_CONF} (mode 600)."

#!/usr/bin/env bash

# This script writes the recursive S3 listing of the BigPicture inbox to
# OUTPUT_DIR (default: current folder), as bpprod-inbox-all.s3.txt or
# bpstaging-inbox-all.s3.txt
set -euo pipefail

# ------------------------------------------------------------------------------
# Usage / Help
# ------------------------------------------------------------------------------
usage() {
    echo "Usage: $0 [-t TARGET] [-o OUTPUT_DIR] [-a MAX_AGE] [-h]"
    echo "  -t  Target: bp-prod or bp-staging (Optional, default: bp-prod)"
    echo "  -o  Output folder (Optional, default: current folder)"
    echo "  -a  Max age in seconds. Keep an existing listing younger than this"
    echo "      instead of regenerating it (Optional, default: always regenerate)"
    echo "  -h  Show this help message"
    exit 1
}

TARGET="bp-prod"
OUTPUT_DIR="."
MAX_AGE=""

while getopts ":t:o:a:h" opt; do
    case "${opt}" in
        t) TARGET="${OPTARG}" ;;
        o) OUTPUT_DIR="${OPTARG}" ;;
        a) MAX_AGE="${OPTARG}" ;;
        h) usage ;;
        :)
            echo "[-] Error: Option -${OPTARG} requires an argument." >&2
            usage
            ;;
        \?)
            echo "[-] Error: Invalid option -${OPTARG}." >&2
            usage
            ;;
    esac
done

case "$TARGET" in
    bp-prod)
        S3_INBOX_BUCKET="s3://inbox"
        S3_LIST_FILENAME="bpprod-inbox-all.s3.txt"
        ;;
    bp-staging)
        S3_INBOX_BUCKET="s3://staging-inbox"
        S3_LIST_FILENAME="bpstaging-inbox-all.s3.txt"
        ;;
    *)
        echo "[-] Error: Unknown target '${TARGET}'. Supported: bp-prod, bp-staging." >&2
        usage
        ;;
esac

if [[ -n "$MAX_AGE" && ! "$MAX_AGE" =~ ^[0-9]+$ ]]; then
    echo "[-] Error: -a MAX_AGE must be a number of seconds." >&2
    usage
fi
if [[ ! -d "$OUTPUT_DIR" ]]; then
    echo "[-] Error: Output folder '$OUTPUT_DIR' does not exist." >&2
    exit 1
fi
S3_LIST_FILE="${OUTPUT_DIR%/}/${S3_LIST_FILENAME}"

if [[ -n "$MAX_AGE" && -f "$S3_LIST_FILE" ]]; then
    file_age=$(($(date +%s) - $(stat -c %Y "$S3_LIST_FILE")))
    if (( file_age <= MAX_AGE )); then
        echo "[+] Using cached ${S3_LIST_FILE} (${file_age}s old, max ${MAX_AGE}s)."
        exit 0
    fi
fi

S3CMD_CONF="$HOME/.sda/s3cmd-bp-master-inbox.conf"

if ! command -v s3cmd &>/dev/null; then
    echo "[-] Error: s3cmd is not in PATH." >&2
    exit 1
fi
if [[ ! -f "$S3CMD_CONF" ]]; then
    echo "[-] Error: $S3CMD_CONF not found!" >&2
    exit 1
fi

# Write to a temp file first so a failed listing does not leave a truncated file
TMP_FILE=$(mktemp "${S3_LIST_FILE}.XXXXXX")
trap 'rm -f "$TMP_FILE"' EXIT

echo "[+] Listing ${S3_INBOX_BUCKET} (${TARGET}) via s3cmd..."
s3cmd -c "$S3CMD_CONF" ls -r "$S3_INBOX_BUCKET" > "$TMP_FILE"
mv "$TMP_FILE" "$S3_LIST_FILE"

echo "[+] Wrote $(wc -l < "$S3_LIST_FILE") line(s) to ${S3_LIST_FILE}"

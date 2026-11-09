#!/usr/bin/env bash

set -euo pipefail

# ------------------------------------------------------------------------------
# Usage / Help Function
# ------------------------------------------------------------------------------
usage() {
    echo "Usage: $0 -d DATASET_FOLDER -n MAIL_UPLOADER_NAME -m MAIL_UPLOADER [-c CONVERSATION_ID]"
    echo "  -d  Dataset folder name (Mandatory)"
    echo "  -n  Mail uploader name (Mandatory)"
    echo "  -m  Mail uploader email (Mandatory)"
    echo "  -c  Ticket conversation id (Optional). When given, after validation has passed and"
    echo "      the dataset is submitted, a reply is sent to that conversation and the tag"
    echo "      '${VALIDATION_TAG}' is added (uses send_email_through_bptickets.py;"
    echo "      TICKET_API_KEY, TICKET_MAILBOX_ID and TICKET_USER_ID come from the environment if"
    echo "      exported, otherwise from ~/.sda/init_bpsubmit_prod.sh)."
    echo ""
    echo "Exit codes: 0 ok, 1 error, 3 dataset submitted but the reply to the conversation FAILED"
    echo "(do not submit again; check the conversation by hand)."
    exit 1
}

# Tag added to the conversation when the validation-passed reply is sent
VALIDATION_TAG="validation successful"
EXIT_REPLY_FAILED=3

# Parse Command-Line Options
DATASET_FOLDER=""
MAIL_UPLOADER_NAME=""
MAIL_UPLOADER=""
CONVERSATION_ID=""

while getopts ":d:n:m:c:h" opt; do
    case "${opt}" in
        d) DATASET_FOLDER="${OPTARG}" ;;
        n) MAIL_UPLOADER_NAME="${OPTARG}" ;;
        m) MAIL_UPLOADER="${OPTARG}" ;;
        c) CONVERSATION_ID="${OPTARG}" ;;
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

# Validate Mandatory Flags
if [[ -z "$DATASET_FOLDER" || -z "$MAIL_UPLOADER_NAME" || -z "$MAIL_UPLOADER" ]]; then
    echo "[-] Error: Missing mandatory arguments." >&2
    usage
fi

if [[ -n "$CONVERSATION_ID" && ! "$CONVERSATION_ID" =~ ^[0-9]+$ ]]; then
    echo "[-] Error: CONVERSATION_ID must be a number, got '${CONVERSATION_ID}'." >&2
    exit 1
fi

# ------------------------------------------------------------------------------
# Check Current Working Directory
# ------------------------------------------------------------------------------
CURRENT_DIR=$(basename "$PWD")
if [[ "$CURRENT_DIR" != "$DATASET_FOLDER" ]]; then
    echo "[-] Warning: Current directory ('$CURRENT_DIR') does not match requested DATASET_FOLDER ('$DATASET_FOLDER'). Aborting execution." >&2
    exit 1
fi

# ------------------------------------------------------------------------------
# Environment Setup & Command Pre-checks
# ------------------------------------------------------------------------------
# TICKET_* variables already set (and exported) in the environment take precedence over
# the ones in the init file, which only supplies the values that are not set.
TICKET_VARS=(TICKET_API_KEY TICKET_PASSWORD TICKET_MAILBOX_ID TICKET_USER_ID TICKET_URL)
SAVED_TICKET_VARS=()
for v in "${TICKET_VARS[@]}"; do
    if [[ -n "${!v:-}" ]]; then
        printf -v "SAVED_${v}" '%s' "${!v}"
        SAVED_TICKET_VARS+=("$v")
    fi
done

if [[ -f "$HOME/.sda/init_bpsubmit_prod.sh" ]]; then
    # shellcheck disable=SC1091
    source "$HOME/.sda/init_bpsubmit_prod.sh"
else
    echo "[-] Error: $HOME/.sda/init_bpsubmit_prod.sh not found!" >&2
    exit 1
fi

# Restore the TICKET_* values that were set in the environment before the init file ran
for v in ${SAVED_TICKET_VARS[@]+"${SAVED_TICKET_VARS[@]}"}; do
    sv="SAVED_${v}"
    export "$v=${!sv}"
done

VALIDATOR_CMD="${validator:-validator}"

SCRIPT_PATH=$(cd "$(dirname "$0")" && pwd)
for helper in get_inbox_filelist.sh get_filelist_for_datasetfolder.sh; do
    if [[ ! -f "$SCRIPT_PATH/$helper" ]]; then
        echo "[-] Error: Script not found: $SCRIPT_PATH/$helper" >&2
        exit 1
    fi
done

# List of all required commands to pre-check
REQUIRED_COMMANDS=(
    "s3cmd"
    "grep"
    "awk"
    "sed"
    "tr"
    "vault"
    "$VALIDATOR_CMD"
)

MISSING_COMMANDS=()
for cmd in "${REQUIRED_COMMANDS[@]}"; do
    if ! command -v "$cmd" &>/dev/null; then
        MISSING_COMMANDS+=("$cmd")
    fi
done

if [[ ${#MISSING_COMMANDS[@]} -gt 0 ]]; then
    echo "[-] Error: The following required commands/tools are missing or not in PATH:" >&2
    for missing_cmd in "${MISSING_COMMANDS[@]}"; do
        echo "    - $missing_cmd" >&2
    done
    exit 1
fi

# Preflight for the reply to the conversation: check it BEFORE anything is submitted
SEND_EMAIL_SCRIPT="$SCRIPT_PATH/send_email_through_bptickets.py"
if [[ -n "$CONVERSATION_ID" ]]; then
    reply_ok=1
    if [[ ! -f "$SEND_EMAIL_SCRIPT" ]]; then
        echo "[-] Error: Script not found: $SEND_EMAIL_SCRIPT" >&2
        reply_ok=0
    fi
    if ! command -v python3 >/dev/null 2>&1; then
        echo "[-] Error: python3 is required to reply to conversation #${CONVERSATION_ID}." >&2
        reply_ok=0
    fi
    if [[ -z "${TICKET_API_KEY:-${TICKET_PASSWORD:-}}" ]]; then
        echo "[-] Error: TICKET_API_KEY is not set (export it, or set it in ~/.sda/init_bpsubmit_prod.sh)." >&2
        reply_ok=0
    fi
    for v in TICKET_MAILBOX_ID TICKET_USER_ID; do
        if ! [[ "${!v:-}" =~ ^[0-9]+$ ]]; then
            echo "[-] Error: ${v} must be set to a number (export it, or set it in ~/.sda/init_bpsubmit_prod.sh)." >&2
            reply_ok=0
        fi
    done
    if [[ $reply_ok -eq 0 ]]; then
        exit 1
    fi
fi

# Reply to the uploader's conversation: validation passed. Returns the script's exit code.
html_escape() {
    local s="$1"
    s="${s//&/&amp;}"
    s="${s//</&lt;}"
    s="${s//>/&gt;}"
    printf '%s' "$s"
}

send_validation_reply() {
    local body_file rc name folder
    body_file=$(mktemp "${TMPDIR:-/tmp}/bpsubmit_reply.XXXXXX") || return 1
    name=$(html_escape "$MAIL_UPLOADER_NAME")
    folder=$(html_escape "$DATASET_FOLDER")
    # HTML body: the dataset folder name is bold
    printf '<p>Dear %s,</p>\n<p>We are pleased to inform you that your submission in the folder <b>%s</b> has successfully passed validation.<br>\nWe will now proceed with the ingestion process and will update you once it is complete.<br>\nPlease let us know if you have any questions in the meantime.</p>\n<p>Best regards,</p>\n<p>BP-Ops Team</p>\n' \
        "$name" "$folder" > "$body_file"
    rc=0
    python3 "$SEND_EMAIL_SCRIPT" --reply-to "$CONVERSATION_ID" --body-file "$body_file" --html \
        --tag "$VALIDATION_TAG" --send < /dev/null || rc=$?
    rm -f "$body_file"
    return $rc
}

# -------------------------
# Vault authentication
# -------------------------
if ! vault token renew >/dev/null 2>&1; then
  echo "Error: You must log in to vault.nbis.se before using this script"
  exit 1
fi


echo "=========================================================================="
echo " Processing Dataset: ${DATASET_FOLDER}"
echo "=========================================================================="

# ------------------------------------------------------------------------------
# Refresh bpprod-inbox-all.s3.txt (Only if missing or older than 1 day / 86400s)
# ------------------------------------------------------------------------------
S3_LIST_FILE="../bpprod-inbox-all.s3.txt"
bash "$SCRIPT_PATH/get_inbox_filelist.sh" -t bp-prod -o .. -a 86400

# ------------------------------------------------------------------------------
# Extract USER_ID
# ------------------------------------------------------------------------------
USER_ID=$(grep "$DATASET_FOLDER" "$S3_LIST_FILE" | head -n 1 | awk '{print $NF}' | awk -F'/' '{print $4}' | sed 's/_lifescience-ri.eu/@lifescience-ri.eu/g' || true)

if [[ -z "$USER_ID" ]]; then
    echo "[-] Error: Could not determine USER_ID for ${DATASET_FOLDER} from ${S3_LIST_FILE}." >&2
    exit 1
fi

echo "[+] Retrieved USER_ID: ${USER_ID}"

# ------------------------------------------------------------------------------
# Step 1: Generate File List
# ------------------------------------------------------------------------------
user_underscore=$(echo "$USER_ID" | tr '@' '_')

echo "[+] Generating file list for user: ${user_underscore}..."
bash "$SCRIPT_PATH/get_filelist_for_datasetfolder.sh" -u "$user_underscore" "$DATASET_FOLDER"

# ------------------------------------------------------------------------------
# Step 2: Dry-run Validation
# ------------------------------------------------------------------------------
echo "[+] Running dry-run validation..."
DRY_RUN_OUTPUT=$("$VALIDATOR_CMD" -c prod \
    -u "$USER_ID" \
    -d "$DATASET_FOLDER" \
    -t "$ACCESS_TOKEN" \
    -n "$MAIL_UPLOADER_NAME" \
    -e "$MAIL_UPLOADER" \
    --dry-run 2>&1)

echo "$DRY_RUN_OUTPUT"

# Verify dry-run output
if ! echo "$DRY_RUN_OUTPUT" | grep -q "All checks passed"; then
    echo "[-] Validation dry-run failed or did not report 'All checks passed'. Aborting." >&2
    exit 1
fi

# ------------------------------------------------------------------------------
# Step 3: Backup XML/XSD Folders & Execute Final Submission
# ------------------------------------------------------------------------------
echo "[+] Dry-run passed. Backing up xml-files and xsd-files..."
mkdir -p bak
if [[ -d "xml-files" ]]; then
    mv xml-files bak/
fi
if [[ -d "xsd-files" ]]; then
    mv xsd-files bak/
fi

echo "[+] Executing production submission..."
"$VALIDATOR_CMD" -c prod \
    -u "$USER_ID" \
    -d "$DATASET_FOLDER" \
    -t "$ACCESS_TOKEN" \
    -n "$MAIL_UPLOADER_NAME" \
    -e "$MAIL_UPLOADER"

# ------------------------------------------------------------------------------
# Tell the uploader: validation passed and the dataset is submitted.
# A failed reply must not abort the rest (the dataset IS submitted): remember it
# and exit with EXIT_REPLY_FAILED at the end.
# ------------------------------------------------------------------------------
REPLY_FAILED=0
if [[ -n "$CONVERSATION_ID" ]]; then
    echo "[+] Replying to conversation #${CONVERSATION_ID} (tag: ${VALIDATION_TAG})..."
    if send_validation_reply; then
        echo "[+] Replied to conversation #${CONVERSATION_ID}."
    else
        REPLY_FAILED=1
        echo "[-] WARNING: the dataset is submitted, but the reply to conversation #${CONVERSATION_ID} FAILED. Do NOT submit again; reply by hand." >&2
    fi
fi

# ------------------------------------------------------------------------------
# Step 4: Extract Metadata & Write Info File
# ------------------------------------------------------------------------------
echo "[+] Retrieving metadata for dataset info file..."

# Extract DATASET_ID from dataset_id.txt (checking root and backup locations)
DATASET_ID=""
if [[ -f "dataset_id.txt" ]]; then
    DATASET_ID=$(tr -d '\r\n' < dataset_id.txt)
elif [[ -f "bak/dataset_id.txt" ]]; then
    DATASET_ID=$(tr -d '\r\n' < bak/dataset_id.txt)
else
    echo "[!] Warning: dataset_id.txt not found." >&2
fi

# Extract MAIL_UPLOADER_ORGANIZATION_NAME from xml-files/organisation.xml
MAIL_UPLOADER_ORGANIZATION_NAME=""
ORG_XML=""
if [[ -f "bak/xml-files/organisation.xml" ]]; then
    ORG_XML="bak/xml-files/organisation.xml"
elif [[ -f "xml-files/organisation.xml" ]]; then
    ORG_XML="xml-files/organisation.xml"
fi

if [[ -n "$ORG_XML" ]]; then
    MAIL_UPLOADER_ORGANIZATION_NAME=$(grep -oP '(?<=<NAME>).*?(?=</NAME>)' "$ORG_XML" 2>/dev/null || sed -n 's/.*<NAME>\(.*\)<\/NAME>.*/\1/p' "$ORG_XML")
else
    echo "[!] Warning: xml-files/organisation.xml not found." >&2
fi

# Write summary file
INFO_FILE="${DATASET_FOLDER}.info.conf"
cat << EOF > "$INFO_FILE"
export DATASET_FOLDER=${DATASET_FOLDER}
export USER_ID=${USER_ID}
export DATASET_ID=${DATASET_ID}
export MAIL_UPLOADER_NAME="${MAIL_UPLOADER_NAME}"
export MAIL_UPLOADER="${MAIL_UPLOADER}"
export MAIL_UPLOADER_ORGANIZATION_NAME="${MAIL_UPLOADER_ORGANIZATION_NAME}"
EOF

echo "[+] Successfully created summary file: ${INFO_FILE}"

if [[ $REPLY_FAILED -eq 1 ]]; then
    exit "$EXIT_REPLY_FAILED"
fi
cat "$INFO_FILE"
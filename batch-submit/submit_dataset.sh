#!/usr/bin/env bash

set -euo pipefail

# ------------------------------------------------------------------------------
# Usage / Help Function
# ------------------------------------------------------------------------------
usage() {
    echo "Usage: $0 -d DATASET_FOLDER -n MAIL_UPLOADER_NAME -m MAIL_UPLOADER [-c CONVERSATION_ID]"
    echo "       $0 -p [-t]"
    echo "  -d  Dataset folder name (Mandatory)"
    echo "  -n  Mail uploader name (Mandatory)"
    echo "  -m  Mail uploader email (Mandatory)"
    echo "  -c  Ticket conversation id (Optional). When given, after validation has passed and"
    echo "      the dataset is submitted, a reply is sent to that conversation and the tag"
    echo "      '${VALIDATION_TAG}' is added (uses send_email_through_bptickets.py;"
    echo "      TICKET_USER_ID must be exported; TICKET_MAILBOX_ID defaults to 2; TICKET_API_KEY is"
    echo "      read from vault unless exported)."
    echo "  -p  Only check the prerequisites (tools, vault login, KUBECONFIG, credentials),"
    echo "      create the missing credential files from vault, and exit. Used by batch_submit.sh."
    echo "  -t  With -p: also check the settings needed to reply to tickets."
    echo ""
    echo "Prerequisites: s3cmd, vault (logged in), kubectl, bpctl, xmllint, crypt4gh, jq, KUBECONFIG, ACCESS_TOKEN."
    echo "Missing ~/.sda/s3cmd-bp-master-inbox.conf and TICKET_API_KEY are fetched from vault."
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
CHECK_ONLY=0
NEED_TICKET=0

while getopts ":d:n:m:c:pth" opt; do
    case "${opt}" in
        d) DATASET_FOLDER="${OPTARG}" ;;
        n) MAIL_UPLOADER_NAME="${OPTARG}" ;;
        m) MAIL_UPLOADER="${OPTARG}" ;;
        c) CONVERSATION_ID="${OPTARG}" ;;
        p) CHECK_ONLY=1 ;;
        t) NEED_TICKET=1 ;;
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
if [[ $CHECK_ONLY -eq 0 && ( -z "$DATASET_FOLDER" || -z "$MAIL_UPLOADER_NAME" || -z "$MAIL_UPLOADER" ) ]]; then
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
if [[ $CHECK_ONLY -eq 0 && "$CURRENT_DIR" != "$DATASET_FOLDER" ]]; then
    echo "[-] Warning: Current directory ('$CURRENT_DIR') does not match requested DATASET_FOLDER ('$DATASET_FOLDER'). Aborting execution." >&2
    exit 1
fi

# ------------------------------------------------------------------------------
# Environment Setup & Command Pre-checks
# ------------------------------------------------------------------------------
SCRIPT_PATH=$(cd "$(dirname "$0")" && pwd)

# The validator of this repository (it lives next to this folder)
VALIDATOR_CMD="$SCRIPT_PATH/../validator/validator.sh"
export VAULT_ADDR="${VAULT_ADDR:-https://vault.nbis.se}"

# Ticket settings: the mailbox defaults to 2, TICKET_USER_ID must be set by each user
TICKET_MAILBOX_ID="${TICKET_MAILBOX_ID:-2}"
export TICKET_MAILBOX_ID

# Admin API token for the validator: from the environment, else from the s3cmd
# download config (~/.sda/s3cmd-bp-download.conf)
DOWNLOAD_CONF="$HOME/.sda/s3cmd-bp-download.conf"
if [[ -z "${ACCESS_TOKEN:-}" && -f "$DOWNLOAD_CONF" ]]; then
    ACCESS_TOKEN=$(grep access_token "$DOWNLOAD_CONF" | head -n 1 | awk '{print $NF}' || true)
fi
export ACCESS_TOKEN="${ACCESS_TOKEN:-}"
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
    "kubectl"
    "bpctl"
    "xmllint"
    "crypt4gh"
    "jq"
)

MISSING_COMMANDS=()
for cmd in "${REQUIRED_COMMANDS[@]}"; do
    if ! command -v "$cmd" &>/dev/null; then
        MISSING_COMMANDS+=("$cmd")
    fi
done

if [[ ! -x "$VALIDATOR_CMD" ]]; then
    MISSING_COMMANDS+=("$VALIDATOR_CMD")
fi

if [[ ${#MISSING_COMMANDS[@]} -gt 0 ]]; then
    echo "[-] Error: The following required commands/tools are missing or not in PATH:" >&2
    for missing_cmd in "${MISSING_COMMANDS[@]}"; do
        echo "    - $missing_cmd" >&2
    done
    exit 1
fi

# ------------------------------------------------------------------------------
# Vault login and credentials
# ------------------------------------------------------------------------------
if ! vault token renew >/dev/null 2>&1 && ! vault token lookup >/dev/null 2>&1; then
    echo "[-] Error: You must log in to ${VAULT_ADDR} (vault login) before using this script." >&2
    exit 1
fi

# ~/.sda/s3cmd-bp-master-inbox.conf is created from vault if it is missing
bash "$SCRIPT_PATH/setup_credentials.sh" || exit 1

# KUBECONFIG is needed by the validator for the production deployment
KUBECONFIG="${KUBECONFIG:-$HOME/.sda/kubeconfig_bp.yaml}"
export KUBECONFIG
if [[ ! -s "$KUBECONFIG" ]]; then
    echo "[-] Error: KUBECONFIG file '$KUBECONFIG' not found. Export KUBECONFIG, or create the file with cluster_v2/k8s_cluster/scripts/get_kubeconfig.sh from BigPicture-Deployment." >&2
    exit 1
fi

# The file must also work: kubectl has to reach the cluster with it
if ! kubectl get ns --request-timeout=20s >/dev/null 2>&1; then
    echo "[-] Error: 'kubectl get ns' failed with KUBECONFIG '$KUBECONFIG' (expired credentials, VPN, wrong file?). Output:" >&2
    kubectl get ns --request-timeout=20s 2>&1 | sed 's/^/    /' >&2 || true
    exit 1
fi

if [[ -z "${ACCESS_TOKEN:-}" ]]; then
    echo "[-] Error: ACCESS_TOKEN is not set (export it, or provide ${DOWNLOAD_CONF} with an access_token line)." >&2
    exit 1
fi

# Preflight for the reply to the conversation: check it BEFORE anything is submitted
SEND_EMAIL_SCRIPT="$SCRIPT_PATH/send_email_through_bptickets.py"
if [[ -n "$CONVERSATION_ID" ]]; then
    NEED_TICKET=1
fi
if [[ $NEED_TICKET -eq 1 ]]; then
    # The API key comes from vault unless it is exported or set in the init file
    if [[ -z "${TICKET_API_KEY:-${TICKET_PASSWORD:-}}" ]]; then
        echo "[+] TICKET_API_KEY is not set, reading it from vault..."
        if TICKET_API_KEY=$(vault kv get -field=apikey bp-secrets/bp-tickets) && [[ -n "$TICKET_API_KEY" ]]; then
            export TICKET_API_KEY
        else
            echo "[-] Error: could not read TICKET_API_KEY from vault (bp-secrets/bp-tickets)." >&2
            exit 1
        fi
    fi
    reply_ok=1
    if [[ ! -f "$SEND_EMAIL_SCRIPT" ]]; then
        echo "[-] Error: Script not found: $SEND_EMAIL_SCRIPT" >&2
        reply_ok=0
    fi
    if ! command -v python3 >/dev/null 2>&1; then
        echo "[-] Error: python3 is required to reply to tickets." >&2
        reply_ok=0
    fi
    for v in TICKET_MAILBOX_ID TICKET_USER_ID; do
        if ! [[ "${!v:-}" =~ ^[0-9]+$ ]]; then
            echo "[-] Error: ${v} must be set to a number (export it; TICKET_USER_ID is your agent id, the number in https://tickets.nbis.se/users/profile/<id>)." >&2
            reply_ok=0
        fi
    done
    if [[ $reply_ok -eq 0 ]]; then
        exit 1
    fi
fi

if [[ $CHECK_ONLY -eq 1 ]]; then
    echo "[+] All prerequisites are met."
    exit 0
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
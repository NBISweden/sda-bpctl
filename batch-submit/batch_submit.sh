#!/usr/bin/env bash

set -uo pipefail

# Exit code of submit_dataset.sh: dataset submitted, but the reply to the conversation failed
EXIT_REPLY_FAILED=3

# ------------------------------------------------------------------------------
# Usage / Help Function
# ------------------------------------------------------------------------------
usage() {
    echo "Usage: $0 LIST_FILE"
    echo ""
    echo "  LIST_FILE  File with one dataset per line, '|' delimited:"
    echo "             DATASET_FOLDER|MAIL_UPLOADER|MAIL_UPLOADER_NAME[|CONVERSATION_ID]"
    echo ""
    echo "  Empty lines and lines starting with '#' are ignored."
    echo "  Each dataset is submitted with submit_dataset.sh inside ./DATASET_FOLDER"
    echo "  (created if missing) and its output is logged to"
    echo "  ./DATASET_FOLDER/submit_dataset.log"
    echo ""
    echo "  CONVERSATION_ID (optional) is the ticket conversation of the uploader (the number in"
    echo "  https://tickets.nbis.se/conversation/<ID>). It is passed to submit_dataset.sh (-c), which"
    echo "  replies to that conversation and adds the tag 'validation successful' once validation"
    echo "  has passed and the dataset is submitted. Without CONVERSATION_ID no reply is sent."
    echo "  The ticket settings (TICKET_MAILBOX_ID, TICKET_USER_ID, optionally TICKET_API_KEY) come"
    echo "  from the environment or ~/.sda/init_bpsubmit_prod.sh. TICKET_API_KEY is read from vault"
    echo "  if it is not set."
    echo ""
    echo "  Before the first dataset the prerequisites are checked (s3cmd, vault login, kubectl,"
    echo "  KUBECONFIG, ACCESS_TOKEN). A missing ~/.sda/s3cmd-bp-master-inbox.conf is created from vault."
    echo ""
    echo "Example LIST_FILE:"
    echo "  DATASET_AAAAAAA|aaaaaaaaa.aaaaaa@dummy.com|Dummy User|889"
    echo "  DATASET_BBBBBBB|bbbbbbbbb.bbbbbb@dummy.com|Other User"
    exit 1
}

if [[ $# -ne 1 || "$1" == "-h" || "$1" == "--help" ]]; then
    usage
fi

LIST_FILE="$1"
if [[ ! -f "$LIST_FILE" ]]; then
    echo "[-] Error: List file '$LIST_FILE' not found." >&2
    exit 1
fi

# Absolute path, since each submission runs after cd into the dataset folder
SCRIPT_PATH=$(cd "$(dirname "$0")" && pwd)
SUBMIT_SCRIPT="${SCRIPT_PATH}/submit_dataset.sh"
if [[ ! -f "$SUBMIT_SCRIPT" ]]; then
    echo "[-] Error: Script not found: $SUBMIT_SCRIPT" >&2
    exit 1
fi

# Check the prerequisites once, before the first dataset (tools, vault login, KUBECONFIG,
# credentials; missing credential files are created from vault)
precheck_args=(-p)
if awk -F'|' '!/^[[:space:]]*(#|$)/ && $4 ~ /[^[:space:]]/ { found=1 } END { exit !found }' "$LIST_FILE"; then
    precheck_args+=(-t)
fi
echo "[batch] Checking prerequisites..."
if ! bash "$SUBMIT_SCRIPT" "${precheck_args[@]}" < /dev/null; then
    echo "[-] Error: prerequisites are not met. Nothing was submitted." >&2
    exit 1
fi

trim() {
    local s="$1"
    s="${s#"${s%%[![:space:]]*}"}"
    s="${s%"${s##*[![:space:]]}"}"
    printf '%s' "$s"
}

WORK_DIR="$PWD"
SUCCEEDED=()
FAILED=()
REPLY_FAILED=()
REPLIED=()

# ------------------------------------------------------------------------------
# Process Each Dataset
# ------------------------------------------------------------------------------
# Read from fd 3 so that commands inside the loop cannot consume the list file
line_no=0
while IFS='|' read -r DATASET_FOLDER MAIL_UPLOADER MAIL_UPLOADER_NAME CONVERSATION_ID _ <&3 || [[ -n "${DATASET_FOLDER:-}" ]]; do
    line_no=$((line_no + 1))
    DATASET_FOLDER=$(trim "${DATASET_FOLDER:-}")
    MAIL_UPLOADER=$(trim "${MAIL_UPLOADER:-}")
    MAIL_UPLOADER_NAME=$(trim "${MAIL_UPLOADER_NAME:-}")
    CONVERSATION_ID=$(trim "${CONVERSATION_ID:-}")

    # Skip empty and comment lines
    if [[ -z "$DATASET_FOLDER" || "$DATASET_FOLDER" == \#* ]]; then
        DATASET_FOLDER=""
        continue
    fi

    if [[ -z "$MAIL_UPLOADER" || -z "$MAIL_UPLOADER_NAME" ]]; then
        echo "[-] Line ${line_no}: missing MAIL_UPLOADER or MAIL_UPLOADER_NAME for '${DATASET_FOLDER}'. Skipping." >&2
        FAILED+=("${DATASET_FOLDER} (line ${line_no}: malformed row)")
        DATASET_FOLDER=""
        continue
    fi

    if [[ -n "$CONVERSATION_ID" && ! "$CONVERSATION_ID" =~ ^[0-9]+$ ]]; then
        echo "[-] Line ${line_no}: CONVERSATION_ID '${CONVERSATION_ID}' for '${DATASET_FOLDER}' is not a number. Skipping." >&2
        FAILED+=("${DATASET_FOLDER} (line ${line_no}: invalid CONVERSATION_ID)")
        DATASET_FOLDER=""
        continue
    fi

    echo "=========================================================================="
    echo " [batch] Submitting ${DATASET_FOLDER} (${MAIL_UPLOADER_NAME} <${MAIL_UPLOADER}>)"
    echo "=========================================================================="

    mkdir -p "${WORK_DIR}/${DATASET_FOLDER}"
    LOG_FILE="${WORK_DIR}/${DATASET_FOLDER}/submit_dataset.log"

    submit_args=(-d "$DATASET_FOLDER" -n "$MAIL_UPLOADER_NAME" -m "$MAIL_UPLOADER")
    if [[ -n "$CONVERSATION_ID" ]]; then
        submit_args+=(-c "$CONVERSATION_ID")
    fi

    # Run in a subshell so that cd and failures do not affect the batch loop
    (
        cd "${WORK_DIR}/${DATASET_FOLDER}" &&
        bash "$SUBMIT_SCRIPT" "${submit_args[@]}"
    ) < /dev/null 2>&1 | tee "$LOG_FILE"
    rc=${PIPESTATUS[0]}

    if [[ $rc -eq 0 ]]; then
        echo "[+] ${DATASET_FOLDER}: submitted successfully."
        SUCCEEDED+=("${DATASET_FOLDER}")
        if [[ -n "$CONVERSATION_ID" ]]; then
            REPLIED+=("${DATASET_FOLDER} (conversation #${CONVERSATION_ID})")
        fi
    elif [[ $rc -eq $EXIT_REPLY_FAILED ]]; then
        echo "[-] ${DATASET_FOLDER}: submitted, but the reply to conversation #${CONVERSATION_ID} FAILED. See ${LOG_FILE}" >&2
        SUCCEEDED+=("${DATASET_FOLDER}")
        REPLY_FAILED+=("${DATASET_FOLDER} (conversation #${CONVERSATION_ID})")
    else
        echo "[-] ${DATASET_FOLDER}: submission FAILED (exit code ${rc}). See ${LOG_FILE}" >&2
        FAILED+=("${DATASET_FOLDER} (exit code ${rc}, log: ${LOG_FILE})")
    fi
    DATASET_FOLDER=""
done 3< "$LIST_FILE"

# ------------------------------------------------------------------------------
# Summary Report
# ------------------------------------------------------------------------------
echo ""
echo "=========================================================================="
echo " Batch submission summary"
echo "=========================================================================="
echo "Succeeded (${#SUCCEEDED[@]}):"
for ds in ${SUCCEEDED[@]+"${SUCCEEDED[@]}"}; do
    echo "    + $ds"
done
echo "Failed (${#FAILED[@]}):"
for ds in ${FAILED[@]+"${FAILED[@]}"}; do
    echo "    - $ds"
done
echo "Replied to conversation (${#REPLIED[@]}):"
for ds in ${REPLIED[@]+"${REPLIED[@]}"}; do
    echo "    + $ds"
done
echo "Submitted, but reply FAILED (${#REPLY_FAILED[@]}):"
for ds in ${REPLY_FAILED[@]+"${REPLY_FAILED[@]}"}; do
    echo "    - $ds"
done

if [[ ${#FAILED[@]} -gt 0 || ${#REPLY_FAILED[@]} -gt 0 ]]; then
    exit 1
fi

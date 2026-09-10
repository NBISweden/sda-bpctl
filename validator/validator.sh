#!/bin/bash

# Resolve absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Set working directory (defaults to current directory if not set)
WORKDIR="${WORKDIR:-$(pwd)}"

# Define terminal reset variable
reset="\033[0m"

# Function for echoing colored text
cecho() {
    local color="$1"
    local text="$2"

    case $color in
        red)     color="\033[31m";;
        green)   color="\033[32m";;
        yellow)  color="\033[33m";;
        blue)    color="\033[34m";;
        magenta) color="\033[35m";;
        *)       color="\033[39m";;
    esac

    # If the output is a terminal, print with colors; else just print text
    if [ -t 2 ]; then
        printf "%b%s%b\n" "$color" "$text" "$reset"
    else
        printf "%s\n" "$text"
    fi
}

# Check if the tools that are needed are installed
if [ ! "$(command -v s3cmd)" ];then
    cecho red "s3cmd command does not exist"
    exit 1
fi

if [ ! "$(command -v vault)" ];then
    cecho red "vault command does not exist"
    exit 1
fi

if [ ! "$(command -v xmllint)" ];then
    cecho red "xmllint command does not exist"
    exit 1
fi

if [ ! "$(command -v crypt4gh)" ];then
    cecho red "crypt4gh command does not exist"
    exit 1
fi

if [ ! "$(command -v kubectl)" ];then
    cecho red "kubectl command does not exist"
    exit 1
fi

if [ ! "$(command -v bpctl)" ];then
    cecho red "bpctl command does not exist"
    exit 1
fi

# Determine which crypt4gh version the user has (python or go)
C4GHGEN=$(crypt4gh generate 2>&1)
if [[ $C4GHGEN != *"the required flag"* ]]; then
    c4gh_decrypt() {
        local sk="$1"
        local file="$2"
        crypt4gh decrypt --sk "$sk" < "$file" > "${file%.c4gh}"
    }
    c4gh_encrypt() {
        local sk="$1"
        local pk="$2"
        local file="$3"
        crypt4gh encrypt --sk "$sk" --recipient_pk "$pk" < "$file" > "$file.c4gh"
    }
else
    c4gh_decrypt() {
        local sk="$1"
        local file="$2"
        crypt4gh decrypt -s "$sk" -f "$file"
    }
    c4gh_encrypt() {
        local sk="$1"
        local pk="$2"
        local file="$3"
        crypt4gh encrypt -s "$sk" -p "$pk" -f "$file"
    }
fi

# OS-specific configurations
if [[ "$OSTYPE" == "darwin"* ]]; then
    NUMFMT="gnumfmt"
    sed_i() { sed -i '' "$@"; }
    sed_i_bak() { sed -i '.bak' "$@"; }
else
    NUMFMT="numfmt"
    sed_i() { sed -i "$@"; }
    sed_i_bak() { sed -i.bak "$@"; }
fi

if [ ! "$(command -v "$NUMFMT")" ];then
    cecho red "$NUMFMT command does not exist. On macOS, you can install it with 'brew install coreutils'."
    exit 1
fi

crypt4gh_required_version="1.9.0"
crypt4gh_current_version=$(crypt4gh -v)

if [ "$(printf '%s\n' "$crypt4gh_required_version" "$crypt4gh_current_version" | sort -V | head -n 1)" != "$crypt4gh_required_version" ]; then
    cecho red "crypt4gh version must be at least $crypt4gh_required_version (found $crypt4gh_current_version)"
    exit 1
fi
INBOX_ACCESS_KEY=""
INBOX_SECRET_KEY=""
HOST_BUCKET=""
INBOX_BUCKETS=()
METADATA_ACCESS_KEY=""
METADATA_SECRET_KEY=""
C4GH_PASSPHRASE=""
METADATA_BUCKET=""
ERROR_STATUS=0
PRIVATE_FOLDER=true
LANDING_PAGE=false
DRY_RUN=false
VALIDATION_ONLY=false
MIN_FILE_SIZE=152
ORG_NAME=""
NAME=""
EMAIL=""
ACCESS_TOKEN=""
version=""
dataset_id=""

function help {
    cat << END_USAGE
    USAGE: $0 -c <cluster> -u <user-id> -d <dataset-name> -n <user-name> -e <user-email> or $0 --clean
    parameters:
    -c, --cluster       Cluster name (prod or staging)
    -u, --user          Username folder in the inbox bucket
    -d, --dataset       Dataset folder (or path) name in the inbox bucket
    -n, --name          The actual name of the uploader
    -e, --email         The actual e-mail of the uploader (for sending email)
    -t, --token         Token for accessing the admin API
    --dry-run           Flag for running only the validation without modifying and moving metadata
    --validation-only   Flag for running only the validation
    --clean             Clean up the files that are created by the script (except the dataset_id.txt file)
END_USAGE
    exit 1
}

# Function for cleaning up the files that are created by the script
function cleanup {
    cecho yellow "Cleaning up ..."

    rm -rf "$WORKDIR/xsd-files" "$WORKDIR/xml-files" "$WORKDIR/PRIVATE" "$WORKDIR/LANDING_PAGE"

    unset C4GH_PASSPHRASE

    rm -f "$WORKDIR/bp_key.pub"

    rm -f "$WORKDIR/general_errors.logs"

    rm -f "$WORKDIR"/*.error

    cecho green "Done"

    exit 0
}

function remove_private_key {
    cecho yellow "Removing private key ..."
    rm -f "$WORKDIR/c4gh.sec.pem"
    cecho green "Private key removed"
}

#parse input
while (( "$#" )); do
    case "$1" in
        -c|--cluster)
            shift
            cluster="$1"
            ;;
        -u|--user)
            shift
            user="$1"
            ;;
        -d|--dataset)
            shift
            dataset="$1"
            ;;
        -n|--name)
            shift
            NAME="$1"
            while [[ -n "$2" && "$2" != -* ]]; do
                NAME="$NAME $2"
                shift
            done
            # Trim leading and trailing whitespace
            NAME="${NAME#"${NAME%%[![:space:]]*}"}"
            NAME="${NAME%"${NAME##*[![:space:]]}"}"
            ;;
        -e|--email)
            shift
            EMAIL="$1"
            ;;
        -t|--token)
            shift
            ACCESS_TOKEN="$1"
            ;;
        --dry-run)
            DRY_RUN=true
            ;;
        --validation-only)
            VALIDATION_ONLY=true
            ;;
        -h|--help)
            help
            ;;
        --clean)
            cleanup
            ;;
    esac
    shift
done

# Check if all arguments are in place
if [ -z "$user" ];then
    cecho red "ERROR: No user given"
    help
fi

if [ -z "$cluster" ];then
    cecho red "ERROR: No cluster given"
    help
fi

if [ -z "$dataset" ];then
    cecho red "ERROR: No dataset given"
    help
fi

if [ "$DRY_RUN" == "false" ] && [ "$VALIDATION_ONLY" == "false" ]; then
    if [ -z "$NAME" ];then
        cecho red "ERROR: No user name given"
        help
    fi

    if [ -z "$EMAIL" ];then
        cecho red "ERROR: No user email given"
        help
    fi

    if [ -z "$ACCESS_TOKEN" ];then
        cecho red "ERROR: No access token given"
        help
    fi
fi

if [ "$DRY_RUN" == "true" ] && [ "$VALIDATION_ONLY" == "true" ]; then
    cecho red "ERROR: cannot use both --dry-run and --validation-only flags"
    help
fi

# Function for removing the leading and trailing "/" from the user and dataset
# and replacing the "@" with "_" in the user if needed
function sanitize_user_dataset {
    user=${user#/}; user=${user%/}; user=${user//@/_}
    dataset=${dataset#/}; dataset=${dataset%/}
}

# Function for getting the xsd files from github repo
function get_xsd_files {
    cecho yellow "Getting xsd files ..."
    mkdir -p "$WORKDIR/xsd-files"

    curl -H "Accept: application/vnd.github.v3+json" \
       https://api.github.com/repos/imi-bigpicture/bigpicture-metaflex/contents/src?ref=$version.0.0 | jq -r '.[] | .download_url' |
    while IFS= read -r url; do
        xsd_name=$(basename "$url")
        curl -o "$WORKDIR/xsd-files/${xsd_name%\?*}" -J -L "$url" >/dev/null 2>&1
    done

    cecho green "Done"
}

# Helper function for making the s3cmd commands in inbox for the prod cluster
# and for all buckets in the staging cluster
function s3cmd_command {
    command s3cmd --host="$HOST_BUCKET" \
    --host-bucket="$HOST_BUCKET" \
    --access_key="$INBOX_ACCESS_KEY" \
    --secret_key="$INBOX_SECRET_KEY" \
    "$@"
}

# Helper function for making the s3cmd commands in metadata bucket for the prod cluster
function s3cmd_metadata {
    command s3cmd --host="$HOST_BUCKET" \
    --host-bucket="$HOST_BUCKET" \
    --access_key="$METADATA_ACCESS_KEY" \
    --secret_key="$METADATA_SECRET_KEY" \
    "$@"
}

function get_credentials {
    if [[ "$cluster" == "prod" ]]; then
        HOST_BUCKET=$(vault kv get -field=endpoints bp-secrets/S3_keys/STO2 | cut -d, -f2)
        INBOX_ACCESS_KEY=$(vault kv get -field=access_key bp-secrets/S3_keys/STO2/inbox)
        INBOX_SECRET_KEY=$(vault kv get -field=secret_key bp-secrets/S3_keys/STO2/inbox)
        INBOX_BUCKETS=("inbox" "inbox-2024-01")
        METADATA_ACCESS_KEY=$(vault kv get -field=access_key bp-secrets/S3_keys/STO2/private)
        METADATA_SECRET_KEY=$(vault kv get -field=secret_key bp-secrets/S3_keys/STO2/private)
        METADATA_BUCKET=$(s3cmd_metadata ls | cut -d'/' -f3)
    elif [[ "$cluster" == "staging" ]]; then
        INBOX_ACCESS_KEY=$(vault kv get -field=access_key bp-secrets/S3_keys/STO2/inbox)
        INBOX_SECRET_KEY=$(vault kv get -field=secret_key bp-secrets/S3_keys/STO2/inbox)
        HOST_BUCKET=$(vault kv get -field=endpoints bp-secrets/S3_keys/STO2 | cut -d, -f2)
        INBOX_BUCKETS=("staging-inbox")
        METADATA_BUCKET="bigpicture-test-metadata"
    else
        cecho red "ERROR: Cluster name is not valid"
        exit 1
    fi

    C4GH_PASSPHRASE=$(vault kv get -field=password bp-secrets/crypt4gh)
}

function validate_private_files {
    cecho yellow "Validating PRIVATE files ..."
    private_files=("$@")
    if [[ $version == "v1" ]]; then
        expected_private_files=("DAC" "submission")
        cecho yellow "Expected private files: ${expected_private_files[*]}"
        if [[ ${#private_files[@]} -ne 2 ]]; then
            cecho red "ERROR: The number of private files is not 2" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
    else
        if [[ ${#private_files[@]} -eq 2 ]]; then
            expected_private_files=("rems" "organisation")
            cecho yellow "Expected private files: ${expected_private_files[*]}"
        elif [[ ${#private_files[@]} -eq 3 ]]; then
            expected_private_files=("rems" "organisation" "datacite")
            cecho yellow "Expected private files: ${expected_private_files[*]}"
        else
            cecho red "ERROR: The number of private files are not 2 or 3" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
    fi

    for private_file in "${expected_private_files[@]}"; do
        local found=false
        for pf in "${private_files[@]}"; do
            if [[ "$pf" == "${private_file}."* ]]; then
                found=true
                break
            fi
        done
        if [[ "$found" == false ]]; then
            cecho red "ERROR: Private file $private_file is missing" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
    done
    cecho green "Done"
}

function validate_files {
    cecho yellow "Validating METADATA folder files ..."
    expected_metadata_content=("dataset" "policy" "image" "annotation" "observation" "sample" "staining")
    inbox_metadata_files=$(for bucket in "${INBOX_BUCKETS[@]}"; do
        s3cmd_command ls "s3://${bucket}/${user}/${dataset}/METADATA/" 2>/dev/null
    done | awk -F'_lifescience-ri.eu/|_elixir-europe.org/' '{print $2}' | cut -d'/' -f3 | sort -u | tr '\n' ' ')
    read -r -a metadata_files <<<"$inbox_metadata_files"
    inbox_private_files=$(for bucket in "${INBOX_BUCKETS[@]}"; do
        s3cmd_command ls "s3://${bucket}/${user}/${dataset}/PRIVATE/" 2>/dev/null
    done | awk -F'_lifescience-ri.eu/|_elixir-europe.org/' '{print $2}' | cut -d'/' -f3 | sort -u | tr '\n' ' ')
    read -r -a metadata_private_files <<<"$inbox_private_files"
    if [[ "$1" == "false" ]]; then
        unset 'expected_metadata_content[3]'
        expected_metadata_content=("${expected_metadata_content[@]}")
        local has_annotation=false
        for mf in "${metadata_files[@]}"; do
            if [[ "$mf" == "annotation"* ]]; then
                has_annotation=true
                break
            fi
        done
        if [[ "$has_annotation" == true ]]; then
            cecho red "ERROR: 'annotation.xml' should not exist because the 'ANNOTATIONS' directory is missing." | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
    fi

    for expected_metadata in "${expected_metadata_content[@]}"; do
        local found=false
        for mf in "${metadata_files[@]}"; do
            if [[ "$mf" == "${expected_metadata}."* ]]; then
                found=true
                break
            fi
        done
        if [[ "$found" == false ]]; then
            cecho red "ERROR: Metadata file $expected_metadata is missing" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
    done

    for metadata_file in "${metadata_files[@]}"; do
        file_basename=$(basename "$metadata_file" | cut -d'.' -f1)
        if [[ "$file_basename" == "observer" ]]; then
            continue
        fi
        local matched=false
        for emc in "${expected_metadata_content[@]}"; do
            if [[ "$metadata_file" == "${emc}."* ]]; then
                matched=true
                break
            fi
        done
        if [[ "$matched" == false ]]; then
            extra_metadata_files+=("$metadata_file")
            cecho red "ERROR: Extra metadata file $metadata_file found" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
    done

    cecho green "Done"

    if [[ "$PRIVATE_FOLDER" == "true" ]]; then
        validate_private_files "${metadata_private_files[@]}"
    fi
}

function validate_structure {
    cecho yellow "Validating folder structure ..."
    annotations=true
    main_folder="$dataset"
    inbox_subfolders=$(for bucket in "${INBOX_BUCKETS[@]}"; do
        s3cmd_command ls "s3://${bucket}/${user}/${dataset}/" 2>/dev/null
    done | awk -F'_lifescience-ri.eu/|_elixir-europe.org/' '{print $2}' | cut -d'/' -f2 | sort -u)
    IFS=$'\n' read -r -d '' -a subfolders <<<"$inbox_subfolders"$'\n'
    expected_subfolders=("METADATA" "IMAGES" "ANNOTATIONS" "PRIVATE" "LANDING_PAGE")
    if [[ "$main_folder" != "DATASET_"* ]]; then
        cecho red "ERROR: Main folder does not start with DATASET_" | tee -a "$WORKDIR/general_errors.logs"
        ERROR_STATUS=1
    fi

    local has_annotations=false
    for sf in "${subfolders[@]}"; do
        if [[ "$sf" == "ANNOTATIONS" ]]; then
            has_annotations=true
            break
        fi
    done
    if [[ "$has_annotations" == false ]]; then
        unset 'expected_subfolders[2]'
        expected_subfolders=("${expected_subfolders[@]}")
        annotations=false
    fi

    local has_landing_page=false
    for sf in "${subfolders[@]}"; do
        if [[ "$sf" == "LANDING_PAGE" ]]; then
            has_landing_page=true
            break
        fi
    done
    if [[ "$has_landing_page" == false ]]; then
        if [[ "$annotations" == "false" ]]; then
            unset 'expected_subfolders[3]'
            expected_subfolders=("${expected_subfolders[@]}")
        else
            unset 'expected_subfolders[4]'
            expected_subfolders=("${expected_subfolders[@]}")
        fi
    else
        thumbnail_files=$(for bucket in "${INBOX_BUCKETS[@]}"; do
            s3cmd_command ls "s3://${bucket}/${user}/${dataset}/LANDING_PAGE/THUMBNAILS/" --recursive 2>/dev/null
        done | awk '{print $4}' | sort -u | wc -l)
        if [[ "$thumbnail_files" -eq 0 ]]; then
            cecho red "ERROR: THUMBNAILS folder is missing or empty" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
        LANDING_PAGE=true
    fi

    if [[ "$LANDING_PAGE" == "true" ]]; then
        landing_page_files=$(for bucket in "${INBOX_BUCKETS[@]}"; do
            s3cmd_command ls "s3://${bucket}/${user}/${dataset}/LANDING_PAGE/" --recursive 2>/dev/null
        done | awk '{print $4}' | sort -u | wc -l)
        if [[ "$landing_page_files" -eq 0 ]]; then
            cecho red "ERROR: LANDING_PAGE folder is empty" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        fi
    fi

    local extra_subfolders=()
    local missing_subfolders=()
    for expected_subfolder in "${expected_subfolders[@]}"; do
        local found=false
        for sf in "${subfolders[@]}"; do
            if [[ "$sf" == "$expected_subfolder" ]]; then
                found=true
                break
            fi
        done
        if [[ "$found" == false ]]; then
            missing_subfolders+=("$expected_subfolder")
            if [[ "$expected_subfolder" == "PRIVATE" ]]; then
                PRIVATE_FOLDER=false
            fi
        fi
    done

    for subfolder in "${subfolders[@]}"; do
        local found=false
        for esf in "${expected_subfolders[@]}"; do
            if [[ "$subfolder" == "$esf" ]]; then
                found=true
                break
            fi
        done
        if [[ "$found" == false ]]; then
            extra_subfolders+=("$subfolder")
        fi
    done

    if [[ ${#extra_subfolders[@]} -ne 0 ]]; then
        cecho red "ERROR: Extra subfolders found: ${extra_subfolders[*]}" | tee -a "$WORKDIR/general_errors.logs"
        ERROR_STATUS=1
    fi

    if [[ ${#missing_subfolders[@]} -ne 0 ]]; then
        cecho red "ERROR: Missing subfolders: ${missing_subfolders[*]}" | tee -a "$WORKDIR/general_errors.logs"
        ERROR_STATUS=1
    fi

    cecho green "Done"

    validate_files "$annotations"
}

# Function for downloading the xml metadata files from the inbox
function get_xml_files {
    mkdir -p "$WORKDIR/xml-files"
    cecho yellow "Getting xml files ..."
    for bucket in "${INBOX_BUCKETS[@]}"; do
        metadata_path=$(s3cmd_command ls "s3://${bucket}/${user}/${dataset}/" 2>/dev/null | grep -i METADATA | awk '{print $2}')
        private_path=$(s3cmd_command ls "s3://${bucket}/${user}/${dataset}/" 2>/dev/null | grep -i PRIVATE | awk '{print $2}')
        if [[ -n "$metadata_path" ]]; then
            s3cmd_command get "$metadata_path" --recursive "$WORKDIR/xml-files/" >/dev/null 2>&1
        fi
        if [[ -n "$private_path" ]]; then
            s3cmd_command get "$private_path" --recursive "$WORKDIR/xml-files/" >/dev/null 2>&1
        fi
    done
    if [[ "$LANDING_PAGE" == "true" ]]; then
        for bucket in "${INBOX_BUCKETS[@]}"; do
            landing_page_path="s3://${bucket}/${user}/${dataset}/LANDING_PAGE/landing_page.xml"
            s3cmd_command get "$landing_page_path" --recursive "$WORKDIR/xml-files/" >/dev/null 2>&1
        done
        if [[ ! -f "$WORKDIR/xml-files/landing_page.xml.c4gh" ]]; then
            cecho red "ERROR: landing_page.xml is missing" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
            LANDING_PAGE=false
        fi
    fi

    if [ -z "$(ls -A "$WORKDIR/xml-files")" ]; then
        cecho red "ERROR: Metadata folder is empty. Check if the path of the metadata folder in inbox is where it should be"
        ERROR_STATUS=1
    fi
    cecho green "Done"
}

# Function for decrypting the xml file
function decrypt_xml_files {
    cecho yellow "Decrypting xml files ..."
    export C4GH_PASSPHRASE
    vault kv get -field=private_key bp-secrets/crypt4gh > "$WORKDIR/c4gh.sec.pem"

    for xml_file in "$WORKDIR"/xml-files/*.c4gh; do
        if ! c4gh_decrypt "$WORKDIR/c4gh.sec.pem" "$xml_file"; then
            cecho red "ERROR: Decryption failed for $xml_file"
            ERROR_STATUS=1
        fi
    done
    rm "$WORKDIR"/xml-files/*.c4gh
    cecho green "Done"
}

# Function for finding the metadata version
function find_metadata_version {
    xml_version=$(xmllint --xpath 'string(/DATASET_SET/DATASET/METADATA_STANDARD)' "$WORKDIR/xml-files/dataset.xml")
    if [[ "$xml_version" == "" ]]; then
        version="v1"
        cecho green "Metadata version is v1"
    else
        cecho green "Metadata version is v2"
        version="v2"
    fi
}

function validate_with_xsd_v1 {
    cecho yellow "Validation started..."
    error_flag=0
    for xml in "$WORKDIR"/xml-files/*; do
        validate="false"
        rootElement=$(xmllint --xpath "name(/*)" "$xml")

        for xsd in "$WORKDIR"/xsd-files/BP.*.xsd; do
            if [[ "$xsd" =~ common|schema ]]; then
                continue
            fi
            topLevelElements=$(xmllint --xpath "/*[local-name()='schema']/*[local-name()='element']/@name" "$xsd" 2>/dev/null | tr ' ' '\n' | cut -d'"' -f2)
            if [[ "$topLevelElements" == *"$rootElement"* ]]; then
                validate="true"
                cecho yellow "Validating $xml with $xsd"
                if ! xmllint --noout --schema "$xsd" "$xml" >/dev/null 2>&1; then
                    xmllint --schema "$xsd" "$xml" > "$WORKDIR/$rootElement.error" 2>&1
                    validate="failed"
                fi
            fi
        done

        if [ "$validate" = "false" ]; then
            for xsd in "$WORKDIR"/xsd-files/*.xsd; do
                if [[ "$(basename "$xsd")" =~ ^BP\.|common|schema ]]; then
                    continue
                fi
                topLevelElements=$(xmllint --xpath "/*[local-name()='schema']/*[local-name()='element']/@name" "$xsd" 2>/dev/null | tr ' ' '\n' | cut -d'"' -f2)
                if [[ "$topLevelElements" == *"$rootElement"* ]]; then
                    validate="true"
                    cecho yellow "Validating $xml with $xsd"
                    if ! xmllint --noout --schema "$xsd" "$xml" >/dev/null 2>&1; then
                        validate="failed"
                    fi
                    break
                fi
            done
        fi

        case $validate in
            false)
                cecho red "ERROR: Validation check is NOT happening for $xml (maybe error on root element)"
                error_flag=1
                ;;
            failed)
                error_flag=1
                ;;
        esac
    done

    if [[ $error_flag -eq 1 ]]; then
        cecho red "Validation failed"
        ERROR_STATUS=1
    else
        cecho green "Validation succeeded"
    fi
}

function validate_with_xsd_v2 {
    cecho yellow "Validation for xml files started..."
    error_flag=0
    for xml in "$WORKDIR"/xml-files/*; do
        xml_base=$(basename "$xml")
        xml_name=$(echo "$xml_base" | cut -d'.' -f1)
        for xsd in "$WORKDIR"/xsd-files/*.xsd; do
            xsd_base=$(basename "$xsd")
            xsd_name=$(echo "$xsd_base" | cut -d'.' -f2)
            if [[ "$xsd_name" == "$xml_name" ]]; then
                cecho yellow "Validating $xml with $xsd"
                if ! xmllint --noout --schema "$xsd" "$xml" >/dev/null 2>&1; then
                    cecho red "ERROR: Validation failed for $xml" | tee -a "$WORKDIR/general_errors.logs"
                    xmllint --noout --schema "$xsd" "$xml" > "$WORKDIR/$xml_name.error" 2>&1

                    error_flag=1
                fi
            fi
        done
    done
    if [[ $error_flag -eq 1 ]]; then
        ERROR_STATUS=1
    else
        cecho green "Validation succeeded for xml files"
    fi
}

# Returns lines in $1 not present in $2.
function check_files {
    comm -23 <(sort <<< "$1") <(sort <<< "$2")
}

# Function for checking that there are no empty files in IMAGES
function check_file_sizes {
    cecho yellow "Checking file sizes ..."
    local bad_files
    bad_files=$(for bucket in "${INBOX_BUCKETS[@]}"; do
        s3cmd_command ls "s3://${bucket}/${user}/${dataset}/" --recursive 2>/dev/null
    done | sort -u | \
        awk -v min="$MIN_FILE_SIZE" '$3+0 <= min {print $4 " (size: " $3 ")"}')
    if [[ -n "$bad_files" ]]; then
        while IFS= read -r line; do
            cecho red "Empty or incomplete file: $line"
        done <<< "$bad_files"
        ERROR_STATUS=1
    fi
}

function comparing_files {
    cecho yellow "Checking files ..."
    all_inbox_files=$(for bucket in "${INBOX_BUCKETS[@]}"; do
        s3cmd_command ls "s3://${bucket}/${user}/${dataset}/" --recursive 2>/dev/null
    done | awk '{print $4}' | sort -u)
    all_inbox_relative=$(echo "$all_inbox_files" | sed -E "s|s3://[^/]+/${user}/${dataset}/||" | sed 's/\.c4gh$//' | sort -u)

    count_inbox_files=$(echo "$all_inbox_relative" | grep -c '^IMAGES/')

    metadata_files_str=""
    for file in "$WORKDIR"/xml-files/*.xml; do
        if [[ "$file" == *"image"* ]]; then
            metadata_files_str=$(xmllint --xpath '/IMAGE_SET/IMAGE/FILES/FILE/@filename' "$file" | awk -F= '{print $2}' | sed 's/"//g')
            break
        fi
    done

    count_metadata_files=$(echo "$metadata_files_str" | wc -l | xargs)
    new_metadata_files=$(echo "$metadata_files_str" | sed 's/\\/\//g')

    if [[ "$metadata_files_str" == "" ]]; then
        cecho red "ERROR: No filenames found in metadata" | tee -a "$WORKDIR/general_errors.logs"
        ERROR_STATUS=1
    elif [ "$count_inbox_files" -lt "$count_metadata_files" ]; then
        cecho red "ERROR: There are more files in metadata than the ones that exist in the inbox (inbox=$count_inbox_files, metadata=$count_metadata_files)" | tee -a "$WORKDIR/general_errors.logs"
        echo "The missing files in the inbox are:"
        missing_inbox_files=$(check_files "$new_metadata_files" "$all_inbox_relative")
        echo "$missing_inbox_files"
        ERROR_STATUS=1
    elif [ "$count_inbox_files" -gt "$count_metadata_files" ]; then
        cecho red "ERROR: There are more files in the inbox than the ones that are referenced in metadata (inbox=$count_inbox_files, metadata=$count_metadata_files)" | tee -a "$WORKDIR/general_errors.logs"
        inbox_images_files=$(echo "$all_inbox_relative" | grep '^IMAGES/IMAGE_')
        extra_inbox_relative=$(check_files "$inbox_images_files" "$new_metadata_files")
        extra_inbox_files=$(awk -F'\t' 'NR==FNR { if (NF) wanted[$1]=1; next } wanted[$2] { print $1 }' \
            <(printf '%s\n' "$extra_inbox_relative") \
            <(paste \
                <(echo "$all_inbox_files" | grep "/IMAGES/") \
            <(echo "$all_inbox_files" | sed -E "s|s3://[^/]+/${user}/${dataset}/||" | sed 's/\.c4gh$//')))
        length_extra_files=$(echo "$extra_inbox_relative" | sed '/^$/d' | wc -l)
        missing_files_diff=$((count_inbox_files - count_metadata_files))
        if [ "$length_extra_files" -eq "$missing_files_diff" ]; then
            echo "The extra files in the inbox are:"
            echo "$extra_inbox_files"
        else
            echo "There are extra files in inbox and other file(s) exist in metadata but not in inbox:"
            echo "$extra_inbox_files"
            ERROR_STATUS=1
        fi
    else
        matching_files=$(check_files "$new_metadata_files" "$all_inbox_relative")
        if [ -n "$matching_files" ]; then
            cecho red "ERROR: The following files were not found in inbox:" | tee -a "$WORKDIR/general_errors.logs"
            echo "$matching_files" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        else
            cecho green "All files listed in the metadata files are present in the inbox"
        fi
    fi

    if [[ "$LANDING_PAGE" == "true" ]]; then
        all_thumbnail_files=$(for bucket in "${INBOX_BUCKETS[@]}"; do
            s3cmd_command ls "s3://${bucket}/${user}/${dataset}/LANDING_PAGE/THUMBNAILS/" --recursive 2>/dev/null
        done | awk '{print $4}' | sort -u)
        all_thumbnail_relative=$(echo "$all_thumbnail_files" | sed -E "s|s3://[^/]+/${user}/${dataset}/||" | sed 's/\.c4gh$//' | sort -u)
        count_inbox_thumbnail_files=$(echo "$all_thumbnail_relative" | sed '/^$/d' | wc -l)
        metadata_thumbnail_files=$(xmllint --xpath '/LANDING_PAGE_SET/LANDING_PAGE/SAMPLE_IMAGE_FILES/SAMPLE_IMAGE_FILE/@filename' "$WORKDIR/xml-files/landing_page.xml" | awk -F= '{print $2}' | sed 's/"//g')
        count_metadata_thumbnail_files=$(echo "$metadata_thumbnail_files" | wc -l)
        if [[ "$metadata_thumbnail_files" == "" ]]; then
            cecho red "ERROR: No filenames found in metadata for thumbnails" | tee -a "$WORKDIR/general_errors.logs"
            ERROR_STATUS=1
        elif [ "$count_inbox_thumbnail_files" -lt "$count_metadata_thumbnail_files" ]; then
            cecho red "ERROR: There are more thumbnail files in metadata than the ones that exist in the inbox (inbox=$count_inbox_thumbnail_files, metadata=$count_metadata_thumbnail_files)" | tee -a "$WORKDIR/general_errors.logs"
            echo "The missing files in the inbox are:"
            missing_inbox_files=$(check_files "$metadata_thumbnail_files" "$all_thumbnail_relative")
            echo "$missing_inbox_files"
            ERROR_STATUS=1
        elif [ "$count_inbox_thumbnail_files" -gt "$count_metadata_thumbnail_files" ]; then
            cecho red "ERROR: There are more thumbnail files in the inbox than the ones that are referenced in metadata (inbox=$count_inbox_thumbnail_files, metadata=$count_metadata_thumbnail_files)" | tee -a "$WORKDIR/general_errors.logs"
            inbox_thumbnail_files="${all_thumbnail_relative//LANDING_PAGE\/THUMBNAILS\//}"
            extra_inbox_relative=$(check_files "$inbox_thumbnail_files" "$metadata_thumbnail_files")
            extra_inbox_files=$(awk -F'\t' 'NR==FNR { if (NF) wanted[$1]=1; next } wanted[$2] { print $1 }' \
                <(printf '%s\n' "$extra_inbox_relative") \
                <(paste \
                    <(echo "$all_thumbnail_files") \
                    <(echo "${all_thumbnail_relative//LANDING_PAGE\/THUMBNAILS\//}")))
            length_extra_files=$(echo "$extra_inbox_relative" | sed '/^$/d' | wc -l)
            missing_files_diff=$((count_inbox_thumbnail_files - count_metadata_thumbnail_files))
            if [ "$length_extra_files" -eq "$missing_files_diff" ]; then
                echo "The extra files in the inbox are:"
                echo "$extra_inbox_files"
            else
                echo "There are extra files in inbox and other file(s) exist in metadata but not in inbox:"
                echo "$extra_inbox_files"
                ERROR_STATUS=1
            fi
        else
            matching_files=$(check_files "$metadata_thumbnail_files" "$all_thumbnail_relative")
            if [ -n "$matching_files" ]; then
                cecho red "ERROR: The following thumbnail files were not found in inbox:" | tee -a "$WORKDIR/general_errors.logs"
                echo "$matching_files" | tee -a "$WORKDIR/general_errors.logs"
                ERROR_STATUS=1
            else
                cecho green "All metadata thumbnail files are present in the inbox"
            fi
        fi
    fi
}

function generate_dataset_id {
    local part_one
    part_one=$(LC_ALL=C tr -dc 'abcdefghjkmnpqrstuvwxyz23456789' </dev/urandom | head -c 6)
    local part_two
    part_two=$(LC_ALL=C tr -dc 'abcdefghjkmnpqrstuvwxyz23456789' </dev/urandom | head -c 6)

    echo "aa-Dataset-$part_one-$part_two"
}

function move_private_metadata {
    cecho yellow "Moving metadata ..."

    stable_id=$(cat "$WORKDIR/dataset_id.txt")
    metadata_inbox_paths=$(for bucket in "${INBOX_BUCKETS[@]}"; do
        s3cmd_command ls "s3://${bucket}/${user}/${dataset}/" 2>/dev/null | grep "PRIVATE" | awk '{print $2}'
    done)
    metadata_bucket_path="s3://${METADATA_BUCKET}/${user}/${stable_id}/${dataset}/PRIVATE/"

    cecho yellow "Moving PRIVATE folder in metadata bucket"

    if [[ "$cluster" == "staging" ]]; then
        while IFS= read -r metadata_inbox_path; do
            [[ -z "$metadata_inbox_path" ]] && continue
            s3cmd_command mv "$metadata_inbox_path" "$metadata_bucket_path" --recursive
        done <<< "$metadata_inbox_paths"
        metadata_size=$(s3cmd_command ls "$metadata_bucket_path" --recursive | awk '{print $3}')
        if [ -z "$metadata_size" ]; then
            cecho red "ERROR: Moving metadata failed"
            exit 1
        fi
    else
        mkdir -p "$WORKDIR/PRIVATE"
        while IFS= read -r metadata_inbox_path; do
            [[ -z "$metadata_inbox_path" ]] && continue
            s3cmd_command get "$metadata_inbox_path" --recursive "$WORKDIR/PRIVATE/" >/dev/null 2>&1
        done <<< "$metadata_inbox_paths"
        s3cmd_metadata put "$WORKDIR/PRIVATE/" "$metadata_bucket_path" --recursive
        metadata_size=$(s3cmd_metadata ls "$metadata_bucket_path" --recursive | awk '{print $3}')
        if [ -z "$metadata_size" ]; then
            cecho red "ERROR: Moving metadata failed"
            exit 1
        fi
        while IFS= read -r metadata_inbox_path; do
            [[ -z "$metadata_inbox_path" ]] && continue
            s3cmd_command del "$metadata_inbox_path" --recursive
        done <<< "$metadata_inbox_paths"
    fi

    cecho green "Done"
}

function modify_dataset {
    cecho yellow "Modifying dataset.xml file ..."

    dataset_id=$(generate_dataset_id)
    echo "$dataset_id" > "$WORKDIR/dataset_id.txt"

    if [ ! -f "$WORKDIR/xml-files/dataset.xml" ]; then
        cecho red "ERROR: dataset.xml file does not exist in xml-files folder"
        exit 1
    else
        sed_i_bak -E "s/(<DATASET[^>]* alias=\"[^\"]*\")/\1 accession=\"$dataset_id\"/g" "$WORKDIR/xml-files/dataset.xml"
    fi

    curl https://raw.githubusercontent.com/NBISweden/EGA-SE-user-docs/main/crypt4gh_bp_key.pub -o "$WORKDIR/bp_key.pub"

    export C4GH_PASSPHRASE
    if ! c4gh_encrypt "$WORKDIR/c4gh.sec.pem" "$WORKDIR/bp_key.pub" "$WORKDIR/xml-files/dataset.xml"; then
        cecho red "ERROR: Encryption failed"
        exit 1
    fi

    for bucket in "${INBOX_BUCKETS[@]}"; do
        s3cmd_command del "s3://${bucket}/${user}/${dataset}/METADATA/dataset.xml.c4gh"
        s3cmd_command put "$WORKDIR/xml-files/dataset.xml.c4gh" "s3://${bucket}/${user}/${dataset}/METADATA/dataset.xml.c4gh"
    done

    cecho green "Done"
}

function organisation_name {
    cecho yellow "Extracting organisation name ..."

    if [ ! -f "$WORKDIR/xml-files/organisation.xml" ]; then
        cecho red "ERROR: organisation.xml file does not exist in the xml-files folder"
        exit 1
    fi

    ORG_NAME=$(xmllint --xpath "//ORGANISATION_SET/ORGANISATION/NAME/text()" "$WORKDIR/xml-files/organisation.xml")
    if [ -z "$ORG_NAME" ]; then
        cecho red "There is no organisation name in xml"
    else
        cecho green "Organisation name: $ORG_NAME"
    fi

    cecho green "Done"
}

function check_kubernetes_access {
    if [[ "$cluster" == "prod" ]]; then
        if ! kubectl -n sda-prod auth can-i create jobs >/dev/null 2>&1; then
            cecho red "ERROR: You do not have access to the 'sda-prod' namespace"
            cecho red "Please check if your KUBECONFIG is correctly set (or VPN)"
            exit 1
        fi
    fi
}

if ! vault token renew >/dev/null 2>&1 && [[ "$1" != "--clean" ]]; then
    cecho red "You must log in to vault.nbis.se before using this script"
    exit 1
fi

if [[ "$1" != "--clean" ]]; then
    if [[ "$DRY_RUN" == false ]] && [[ "$VALIDATION_ONLY" == false ]]; then
        check_kubernetes_access
    fi

    get_credentials

    sanitize_user_dataset
fi

# Validation steps
check_file_sizes

validate_structure

get_xml_files

trap remove_private_key EXIT HUP INT QUIT PIPE TERM

decrypt_xml_files

find_metadata_version

get_xsd_files

"validate_with_xsd_$version"

comparing_files

if [[ "$ERROR_STATUS" -eq 0 ]]; then
    cecho green "All checks passed"

    if [[ "$DRY_RUN" == "true" ]]; then
        cecho yellow "DRY RUN MODE"
        exit 0
    fi

    modify_dataset

    move_private_metadata

    organisation_name
else
    cecho red "Validation failed"
    exit $ERROR_STATUS
fi

cecho green "VALIDATION SUCCESSFUL !!!"
cecho blue "Dataset stable ID: $dataset_id"

if [[ "$VALIDATION_ONLY" == "true" ]]; then
    cecho yellow "VALIDATION ONLY MODE"
    exit 0
fi

if [[ "$cluster" == "staging" ]]; then
    cecho magenta "Kubernetes job NOT deployed for staging cluster"
    exit 0
fi

cat << EOF

     --------------------
    | Setting up k8s job |
     --------------------

EOF

# Create data directory in WORKDIR
mkdir -p "$WORKDIR/data/xml"

# Copy metadata in WORKDIR/data folder
cp -f "$WORKDIR/xml-files/rems.xml" "$WORKDIR/data/xml/rems.txt" || exit 1
cp -f "$WORKDIR/xml-files/policy.xml" "$WORKDIR/data/xml/policy.txt" || exit 1
cp -f "$WORKDIR/xml-files/dataset.xml" "$WORKDIR/data/xml/dataset.txt" || exit 1

# Create the config file in WORKDIR
cp -f "$SCRIPT_DIR/../config.yaml.example" "$WORKDIR/config.yaml"
cp -f "$SCRIPT_DIR/../kustomization.yaml" "$WORKDIR/kustomization.yaml"

# Update the config file
sed_i "s|USER_ID:.*|USER_ID: \"${user//_/@}\"|" "$WORKDIR/config.yaml"
sed_i "s|DATASET_ID:.*|DATASET_ID: \"$dataset_id\"|" "$WORKDIR/config.yaml"
sed_i "s|DATASET_FOLDER:.*|DATASET_FOLDER: \"$dataset\"|" "$WORKDIR/config.yaml"
sed_i "s|CLIENT_ACCESS_TOKEN:.*|CLIENT_ACCESS_TOKEN: \"$ACCESS_TOKEN\"|" "$WORKDIR/config.yaml"
sed_i "s|MAIL_UPLOADER:.*|MAIL_UPLOADER: \"$EMAIL\"|" "$WORKDIR/config.yaml"
sed_i "s|MAIL_UPLOADER_NAME:.*|MAIL_UPLOADER_NAME: \"$NAME\"|" "$WORKDIR/config.yaml"
sed_i "s|MAIL_UPLOADER_ORGANIZATION_NAME:.*|MAIL_UPLOADER_ORGANIZATION_NAME: \"$ORG_NAME\"|" "$WORKDIR/config.yaml"

pushd "$WORKDIR" > /dev/null || exit
bpctl render -x
kubectl kustomize . -o "$dataset".yaml
kubectl -n sda-prod apply --server-side -f "$dataset".yaml
popd > /dev/null || exit

trap - EXIT
remove_private_key
exit $ERROR_STATUS
#!/bin/bash

# This script retrieves the list of files from the S3inbox for a dataset folder.

SCRIPT_PATH=$(dirname "$(readlink -f "$0")")
s3cmdFile=$HOME/.sda/s3cmd-bp-master-inbox.conf
s3cmdFile=$(realpath "$s3cmdFile")
server="prod"
user_underscore=""
dataset_folder="" # Initialize to empty string

Usage="""
Usage: $0 [OPTIONS] <dataset_folder>
Options:
    -h, --help      Show this help message and exit
    -u, --user      Specify the user (default: bp)
    -s3cmd, --s3cmd Specify the s3cmd config file (default: $HOME/.sda/s3cmd-bp-master-inbox.conf)
    -server, --server   Specify the server [prod or staging] (default: prod)
"""

# Handle help flag before parsing arguments to avoid issues
for arg in "$@"; do
  case "$arg" in
    -h|--help)
      echo "$Usage"
      exit 0
      ;;
  esac
done

# This loop parses all options and their arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -u|--user)
            if [ -z "$2" ]; then echo "Error: -u requires an argument"; exit 1; fi
            user_underscore="$2"
            shift 2
            ;;
        -s3cmd|--s3cmd)
            if [ -z "$2" ]; then echo "Error: -s3cmd requires an argument"; exit 1; fi
            s3cmdFile="$2"
            s3cmdFile=$(realpath "$s3cmdFile")
            shift 2
            ;;
        -server|--server)
            if [ -z "$2" ]; then echo "Error: -server requires an argument"; exit 1; fi
            server="$2"
            shift 2
            ;;
        -*) # Handle any other unknown options
            echo "Error: Unknown option $1"
            echo "$Usage"
            exit 1
            ;;
        *) # All remaining arguments are treated as the dataset folder
            if [ -z "$dataset_folder" ]; then
                dataset_folder="$1"
            else
                echo "Error: Only one dataset folder is allowed."
                exit 1
            fi
            shift
            ;;
    esac
done

if [ -z "$dataset_folder" ]; then
    echo "Error: Dataset folder is required."
    echo "$Usage"
    exit 1
fi

if [[ "$server" != "prod" && "$server" != "staging" ]]; then
    echo "Error: Invalid server specified. Must be one of: prod, staging"
    exit 1
fi

if [ ! -f "$s3cmdFile" ]; then
    echo "Error: s3cmd config file $s3cmdFile does not exist."
    exit 1
fi

if [ "$server" == "prod" ]; then
    bucket="inbox"
elif [ "$server" == "staging" ]; then
    bucket="staging-inbox"
else
    # This block is redundant due to the previous check, but good for clarity
    echo "Error: Invalid server setting."
    exit 1
fi

if [ -z "$user_underscore" ]; then
    s3cmd -c "$s3cmdFile" ls "s3://$bucket" --recursive | grep "$dataset_folder" > "$dataset_folder.filelist.txt"
else
    s3cmd -c "$s3cmdFile" ls "s3://$bucket/$user_underscore/$dataset_folder" --recursive > "$dataset_folder.filelist.txt"
fi

wc -l "$dataset_folder.filelist.txt" | awk '{print "Number of files: " $1}'
echo "File list saved to: $dataset_folder.filelist.txt"

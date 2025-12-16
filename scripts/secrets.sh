#!/usr/bin/env bash
# This can be used as a helper to provision secrets consumed by job.yaml
# expected env variables to be set $DB_USER, $DB_NAME, $DB_SCHEMA, $DB_HOST, $DB_PASSWORD, $DB_PORT, $DB_SSL_MODE
# supply the kubernetes namespace as 
set -euo pipefail

SECRET_NAME="sda-sda-svc-bpctl"
if [[ -z "${1:-}" ]]; then
  echo "ERROR: no namespace supplied"
  echo "USAGE: ./secrets.sh <NAMESPACE>"
  exit 1
fi
NAMESPACE=$1

required_envs=(
  NAMESPACE
  DB_USER
  DB_NAME
  DB_SCHEMA
  DB_HOST
  DB_PASSWORD
  DB_PORT
  DB_SSL_MODE
)

echo "Validating required environment variables..."

missing=false
for var in "${required_envs[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "ERROR: Environment variable '$var' is not set."
    missing=true
  else
    echo "$var is set"
  fi
done

if [[ "$missing" == true ]]; then
  echo "🚫 Aborting: One or more required environment variables are missing."
  exit 1
fi

echo "creating: $SECRET_NAME ..."

# Use kubectl apply so rerunning is idempotent
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: $SECRET_NAME
  namespace: $NAMESPACE
type: Opaque
stringData:
  DB_USER: "$DB_USER"
  DB_NAME: "$DB_NAME"
  DB_SCHEMA: "$DB_SCHEMA"
  DB_HOST: "$DB_HOST"
  DB_PASSWORD: "$DB_PASSWORD"
  DB_PORT: "$DB_PORT"
  DB_SSL_MODE: "$DB_SSL_MODE"
EOF

rc=$?

if [[ $rc -eq 0 ]]; then
  echo "success!"
else
  echo "ERROR: Failed to create/update secret (exit code $rc)."
fi

exit $rc


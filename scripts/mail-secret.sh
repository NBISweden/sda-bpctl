#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${1:-}" ]]; then
  echo "ERROR: no namespace supplied"
  echo "USAGE: ./secrets.sh <NAMESPACE>"
  exit 1
fi
NAMESPACE=$1

if [[ $NAMESPACE == "sda-prod" ]]; then
  SECRET_NAME="sda-bpctl-mail"
elif [[ $NAMESPACE == "sda-staging" ]]; then
  SECRET_NAME="pipeline-bpctl-mail"
else
  echo "namespace not recognized, need; sda-prod/sda-staging"
  exit 1
fi

required_envs=(
  NAMESPACE
  MAIL_ADDRESS
  MAIL_PASSWORD
  MAIL_SMTP_HOST
  MAIL_SMTP_PORT
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
  echo "Aborting: One or more required environment variables are missing."
  exit 1
fi

echo "creating: $SECRET_NAME ..."

# Use kubectl apply to ensure idempotent operation
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: $SECRET_NAME
  namespace: $NAMESPACE
type: Opaque
stringData:
  MAIL_ADDRESS: "$MAIL_ADDRESS"
  MAIL_PASSWORD: "$MAIL_PASSWORD"
  MAIL_SMTP_HOST: "$MAIL_SMTP_HOST"
  MAIL_SMTP_PORT: "$MAIL_SMTP_PORT"
EOF

rc=$?

if [[ $rc -eq 0 ]]; then
  echo "success!"
else
  echo "ERROR: Failed to create/update secret (exit code $rc)."
fi

exit $rc

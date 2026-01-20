#!/usr/bin/env bash
set -euo pipefail

export AWS_DEFAULT_REGION=us-east-1

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

usage() {
  cat <<EOF
Usage: $0 <primary-queue-name> [dead-letter-queue-name]
  <primary-queue-name>       Name of the primary SQS queue to create
  [dead-letter-queue-name]   (optional) Name of the dead-letter queue
EOF
  exit 1
}

# Trap errors and print a red error message
trap 'echo -e "${RED}Error: command failed. Exiting.${NC}" >&2' ERR

# Require awslocal
if ! command -v awslocal &>/dev/null; then
  echo -e "${RED}Error:${NC} 'awslocal' not found in PATH. Please install awscli-local.${NC}" >&2
  exit 1
fi

# Validate arguments
if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
fi

PRIMARY_QUEUE="$1"
DLQ_QUEUE="${2-}"

if [[ -n "$DLQ_QUEUE" ]]; then
  echo "Creating dead-letter queue: $DLQ_QUEUE"
  DLQ_URL=$(awslocal sqs create-queue \
    --queue-name "$DLQ_QUEUE" \
    --output text --query 'QueueUrl')
  echo -e "${GREEN}Success:${NC} Dead-letter queue '$DLQ_QUEUE' created at '$DLQ_URL'."

  DLQ_ARN=$(awslocal sqs get-queue-attributes \
    --queue-url "$DLQ_URL" \
    --attribute-names QueueArn \
    --output text --query 'Attributes.QueueArn')
  echo "DLQ ARN: $DLQ_ARN"

  echo "Creating primary queue: $PRIMARY_QUEUE with RedrivePolicy"
  JSON_PAYLOAD=$(cat <<EOF
{
  "QueueName": "$PRIMARY_QUEUE",
  "Attributes": {
    "RedrivePolicy": "{\"deadLetterTargetArn\":\"$DLQ_ARN\",\"maxReceiveCount\":\"3\"}"
  }
}
EOF
)
  awslocal sqs create-queue --cli-input-json "$JSON_PAYLOAD"
  echo -e "${GREEN}Success:${NC} Primary queue '$PRIMARY_QUEUE' created with DLQ '$DLQ_QUEUE'."
else
  echo "Creating primary queue: $PRIMARY_QUEUE"
  JSON_PAYLOAD=$(cat <<EOF
{
  "QueueName": "$PRIMARY_QUEUE"
}
EOF
)
  awslocal sqs create-queue --cli-input-json "$JSON_PAYLOAD"
  echo -e "${GREEN}Success:${NC} Primary queue '$PRIMARY_QUEUE' created."
fi

echo -e "${GREEN}All done.${NC}"

export AWS_DEFAULT_REGION=ap-south-1

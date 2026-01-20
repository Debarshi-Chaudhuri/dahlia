#!/usr/bin/env bash
set -euo pipefail

# Batch-create SQS queues with optional DLQs
# Requires create-queue.sh in the same directory

# Determine script directory (works under bash, sh, or zsh)
SCRIPT_PATH="${BASH_SOURCE[0]:-$0}"
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd)"
CREATE_SCRIPT="$SCRIPT_DIR/create-queue.sh"

# Ensure create-queue.sh exists
if [[ ! -f "$CREATE_SCRIPT" ]]; then
  echo "[ERROR] create-queue.sh not found in $SCRIPT_DIR" >&2
  exit 1
fi

# Decide how to invoke create-queue.sh
if [[ -x "$CREATE_SCRIPT" ]]; then
  CREATE_CMD=("$CREATE_SCRIPT")
else
  echo "[WARN] create-queue.sh is not executable; running with bash"
  CREATE_CMD=("bash" "$CREATE_SCRIPT")
fi

# Define queue pairs: "<PrimaryQueue> [DLQQueue]"
# Add your queues here
QUEUE_LIST=(
"executor-queue executor-dlq"
)

# Iterate and create queues
for entry in "${QUEUE_LIST[@]}"; do
  # Extract primary and DLQ (if any)
  PRIMARY="${entry%% *}"
  DLQ="${entry#* }"
  # If no DLQ specified, DLQ will equal PRIMARY, so clear it
  if [[ "$PRIMARY" == "$DLQ" ]]; then
    DLQ=""
  fi

  if [[ -n "$DLQ" ]]; then
    echo "[INFO] Creating queue '$PRIMARY' with DLQ '$DLQ'"
    "${CREATE_CMD[@]}" "$PRIMARY" "$DLQ"
  else
    echo "[INFO] Creating queue '$PRIMARY'"
    "${CREATE_CMD[@]}" "$PRIMARY"
  fi
done

echo -e "\n[OK] All queues processed."

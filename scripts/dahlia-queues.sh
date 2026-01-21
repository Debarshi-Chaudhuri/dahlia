#!/usr/bin/env bash
set -uo pipefail

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

# Helper function to create queue and handle QueueAlreadyExists
create_queue_if_not_exists() {
  local output
  if output=$("${CREATE_CMD[@]}" "$@" 2>&1); then
    return 0
  elif echo "$output" | grep -q "QueueAlreadyExists"; then
    echo "[INFO] Queue already exists, skipping..."
    return 0
  else
    echo "$output" >&2
    return 1
  fi
}

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
    create_queue_if_not_exists "$PRIMARY" "$DLQ"
  else
    echo "[INFO] Creating queue '$PRIMARY'"
    create_queue_if_not_exists "$PRIMARY"
  fi
done

echo -e "\n[OK] All queues processed."

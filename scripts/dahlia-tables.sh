#!/usr/bin/env bash
set -euo pipefail

# Batch-create DynamoDB tables for Dahlia Alerts
# Requires create-table.sh in the same directory

# Determine script directory
SCRIPT_PATH="${BASH_SOURCE[0]:-$0}"
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd)"
CREATE_SCRIPT="$SCRIPT_DIR/create-table.sh"

# Check for create-table.sh
if [[ ! -f "$CREATE_SCRIPT" ]]; then
  echo "[ERROR] create-table.sh not found in $SCRIPT_DIR" >&2
  exit 1
fi

# Determine how to run create-table.sh
if [[ -x "$CREATE_SCRIPT" ]]; then
  CREATE_CMD=("$CREATE_SCRIPT")
else
  echo "[WARN] create-table.sh is not executable; running with bash"
  CREATE_CMD=("bash" "$CREATE_SCRIPT")
fi

# Configurable endpoint and throughput
ENDPOINT="http://localhost:9000"
READ_CAPACITY=5
WRITE_CAPACITY=5

# Table 1: signals with 2 GSIs
echo "[INFO] Creating table 'signals'"
"${CREATE_CMD[@]}" -t signals -p signal_id:S -s timestamp:S \
  -g signal_type_org_id_index:signal_type_org_id:S:timestamp:S \
  -g signal_type_index:signal_type:S:timestamp:S \
  -r "$READ_CAPACITY" -w "$WRITE_CAPACITY" -e "$ENDPOINT"

# Table 2: workflows with 1 GSI
echo "[INFO] Creating table 'workflows'"
"${CREATE_CMD[@]}" -t workflows -p workflow_id:S -s version:N \
  -g signal_type_index:signal_type:S:workflow_id:S \
  -r "$READ_CAPACITY" -w "$WRITE_CAPACITY" -e "$ENDPOINT"

# Table 3: workflow_runs with 2 GSIs
echo "[INFO] Creating table 'workflow_runs'"
"${CREATE_CMD[@]}" -t workflow_runs -p run_id:S -s created_at:N \
  -g workflow_id_index:workflow_id:S:created_at:N \
  -g status_index:status:S:created_at:N \
  -r "$READ_CAPACITY" -w "$WRITE_CAPACITY" -e "$ENDPOINT"

# Table 4: action_logs (no GSIs)
echo "[INFO] Creating table 'action_logs'"
"${CREATE_CMD[@]}" -t action_logs -p run_id_action_index:S -s executed_at:N \
  -r "$READ_CAPACITY" -w "$WRITE_CAPACITY" -e "$ENDPOINT"

# Table 5: scheduled_jobs with 1 GSI
echo "[INFO] Creating table 'scheduled_jobs'"
"${CREATE_CMD[@]}" -t scheduled_jobs -p job_id:S \
  -g status_index:status:S:execute_at:N \
  -r "$READ_CAPACITY" -w "$WRITE_CAPACITY" -e "$ENDPOINT"

echo -e "\n[OK] All DynamoDB tables created."
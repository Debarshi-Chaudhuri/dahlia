#!/usr/bin/env bash
set -euo pipefail

# create-table.sh
# Bash script to create a DynamoDB table with optional sort key and multiple GSIs using JSON input
# bash create-table.sh \
#        -t MultiGSIExample \
#        -p id:S \
#        -s createdAt:N \
#        -g GSI1:pk1:S:sk1:N \
#        -g GSI2:pk2:S:sk2:N \
#        -r 5 \
#        -w 5 \
#        -e http://localhost:9000

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

usage() {
  cat <<EOF
Usage: $0 -t TableName -p pkName:pkType [-s skName:skType] \
       [-g gsiName:pkName:pkType[:skName:skType]]... \
       [-r readCapacity] [-w writeCapacity] [-e endpointURL]

  -t  Table name (required)
  -p  Partition key (name:type, e.g. id:S)
  -s  Sort key (optional, name:type, e.g. ts:N)
  -g  Global secondary index definition. Repeatable:
       name:pk:type[:sk:type]
  -r  Read capacity units (default: 5)
  -w  Write capacity units (default: 5)
  -e  DynamoDB endpoint URL (e.g. http://localhost:9000)
EOF
  exit 1
}

# Defaults
RCU=5
WCU=5
ENDPOINT=""
# Collect keys and GSIs
declare PKN PKT SKN SKT
GSI_NAMES=()
GSI_PKS=()
GSI_PKTS=()
GSI_SKS=()
GSI_SKTS=()

# Parse args
while getopts ":t:p:s:g:r:w:e:" opt; do
  case $opt in
    t) TABLE_NAME=$OPTARG;;
    p) IFS=":" read -r PKN PKT <<< "$OPTARG";;
    s) IFS=":" read -r SKN SKT <<< "$OPTARG";;
    g) 
       # Parse GSI definition more carefully
       IFS=":" read -r name gpk gpkt gsk gskt <<< "$OPTARG"
       GSI_NAMES+=("$name")
       GSI_PKS+=("$gpk") 
       GSI_PKTS+=("$gpkt")
       GSI_SKS+=("${gsk:-}")
       GSI_SKTS+=("${gskt:-}")
       ;;
    r) RCU=$OPTARG;;
    w) WCU=$OPTARG;;
    e) ENDPOINT=$OPTARG;;
    *) usage;;
  esac
done

# Validate required
[[ -z "${TABLE_NAME:-}" || -z "${PKN:-}" ]] && usage

# Collect all unique attributes using arrays instead of associative arrays
ALL_ATTR_NAMES=()
ALL_ATTR_TYPES=()

# Function to add attribute if not already present
add_attr() {
  local name="$1"
  local type="$2"
  local i
  
  # Check if attribute already exists
  for i in "${!ALL_ATTR_NAMES[@]}"; do
    if [[ "${ALL_ATTR_NAMES[i]}" == "$name" ]]; then
      return 0  # Already exists
    fi
  done
  
  # Add new attribute
  ALL_ATTR_NAMES+=("$name")
  ALL_ATTR_TYPES+=("$type")
}

# Add table attributes
add_attr "$PKN" "$PKT"
[[ -n "${SKN:-}" ]] && add_attr "$SKN" "$SKT"

# Add GSI attributes
for i in "${!GSI_NAMES[@]}"; do
  add_attr "${GSI_PKS[i]}" "${GSI_PKTS[i]}"
  [[ -n "${GSI_SKS[i]:-}" ]] && add_attr "${GSI_SKS[i]}" "${GSI_SKTS[i]}"
done

# Build JSON
tmp=$(mktemp)

# Start building the JSON
{
  echo "{"
  echo "  \"TableName\": \"${TABLE_NAME}\","
  
  # Attribute definitions
  echo "  \"AttributeDefinitions\": ["
  for i in "${!ALL_ATTR_NAMES[@]}"; do
    [[ $i -gt 0 ]] && echo ","
    echo -n "    {\"AttributeName\":\"${ALL_ATTR_NAMES[i]}\",\"AttributeType\":\"${ALL_ATTR_TYPES[i]}\"}"
  done
  echo ""
  echo "  ],"
  
  # Key schema
  echo "  \"KeySchema\": ["
  echo -n "    {\"AttributeName\":\"${PKN}\",\"KeyType\":\"HASH\"}"
  [[ -n "${SKN:-}" ]] && echo "," && echo -n "    {\"AttributeName\":\"${SKN}\",\"KeyType\":\"RANGE\"}"
  echo ""
  echo "  ],"
  
  # Provisioned throughput
  echo "  \"ProvisionedThroughput\": {"
  echo "    \"ReadCapacityUnits\": ${RCU},"
  echo "    \"WriteCapacityUnits\": ${WCU}"
  echo "  }"
  
  # GSIs if any
  if (( ${#GSI_NAMES[@]} > 0 )); then
    echo ","
    echo "  \"GlobalSecondaryIndexes\": ["
    for i in "${!GSI_NAMES[@]}"; do
      [[ $i -gt 0 ]] && echo ","
      echo "    {"
      echo "      \"IndexName\": \"${GSI_NAMES[i]}\","
      echo "      \"KeySchema\": ["
      echo -n "        {\"AttributeName\":\"${GSI_PKS[i]}\",\"KeyType\":\"HASH\"}"
      [[ -n "${GSI_SKS[i]}" ]] && echo "," && echo -n "        {\"AttributeName\":\"${GSI_SKS[i]}\",\"KeyType\":\"RANGE\"}"
      echo ""
      echo "      ],"
      echo "      \"Projection\": {\"ProjectionType\": \"ALL\"},"
      echo "      \"ProvisionedThroughput\": {"
      echo "        \"ReadCapacityUnits\": ${RCU},"
      echo "        \"WriteCapacityUnits\": ${WCU}"
      echo "      }"
      echo -n "    }"
    done
    echo ""
    echo "  ]"
  fi
  
  echo "}"
} > "$tmp"

# Validate JSON if python is available
if command -v python3 &>/dev/null; then
  if ! python3 -m json.tool "$tmp" > /dev/null 2>&1; then
    echo -e "${RED}[ERROR] Generated JSON is invalid!${NC}" >&2
    exit 1
  fi
elif command -v python &>/dev/null; then
  if ! python -m json.tool "$tmp" > /dev/null 2>&1; then
    echo -e "${RED}[ERROR] Generated JSON is invalid!${NC}" >&2
    exit 1
  fi
fi

CLI=awslocal

# Execute
echo -e "${BLUE}[INFO] Creating table $TABLE_NAME...${NC}"
$CLI dynamodb  --no-cli-pager create-table --cli-input-json file://"$tmp" ${ENDPOINT:+--endpoint-url "$ENDPOINT"} > /dev/null
rc=$?

# Cleanup
rm -f "$tmp"

if [[ $rc -eq 0 ]]; then
  echo -e "${GREEN}[SUCCESS] Table '$TABLE_NAME' created.${NC}"
else
  echo -e "${RED}[ERROR] Failed with exit code $rc${NC}" >&2
  exit $rc
fi
#!/usr/bin/env bash
set -euo pipefail

# 多云监控主线轻量回归（macOS/Linux）
# 核心目标：固定校验健康检查、降级仪表盘、云账号列表和已启用账号的首个资源/概览接口。

usage() {
  cat <<'EOF'
用法:
  scripts/ops/cloud-mainline-check.sh \
    --base-url http://localhost:8082 \
    --output-file ../reports/cloud-mainline-check-2026-05-12.json

参数:
  --base-url URL       API 地址，默认 http://localhost:8082
  --output-file FILE   输出文件，默认 ../reports/cloud-mainline-check-YYYY-MM-DD.json
  --max-accounts NUM   最多抽样检查的启用云账号数，默认 5
  -h, --help           显示帮助
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 2
  fi
}

http_code() {
	local url="$1"
	local out_file="$2"
	local code
	code="$(curl -sS -o "$out_file" -w '%{http_code}' -X GET "$url" || true)"
	if [[ ! "$code" =~ ^[0-9]+$ ]] || [[ "$code" == "000" ]]; then
		code="0"
		if [[ ! -s "$out_file" ]]; then
			jq -n --arg error "请求失败或服务不可达" '{error:$error}' >"$out_file"
		fi
	fi
	echo "$code"
}

url_encode() {
  jq -rn --arg value "$1" '$value|@uri'
}

json_get() {
  local file="$1"
  local expr="$2"
  jq -r "$expr" "$file" 2>/dev/null || true
}

append_cloud_check() {
  local provider="$1"
  local account_id="$2"
  local account_name="$3"
  local resource="$4"
  local action="$5"
  local code="$6"
  local body_file="$7"
  local region="${8:-}"
  local resource_id="${9:-}"
  local url="${10:-}"
  local message
  message="$(json_get "$body_file" '.error // .message // .status // empty')"
  jq -n \
    --arg provider "$provider" \
    --arg accountId "$account_id" \
    --arg accountName "$account_name" \
    --arg resource "$resource" \
    --arg action "$action" \
    --arg region "$region" \
    --arg resourceId "$resource_id" \
    --arg url "$url" \
    --arg message "$message" \
    --argjson code "${code:-0}" \
    '{
      provider: $provider,
      accountId: $accountId,
      accountName: $accountName,
      resource: $resource,
      action: $action,
      region: $region,
      resourceId: $resourceId,
      url: $url,
      code: $code,
      ok: ($code == 200),
      message: $message
    }' >>"$CLOUD_CHECKS_FILE"
}

require_cmd curl
require_cmd jq

BASE_URL="http://localhost:8082"
DATE_TAG="$(date +%F)"
OUTPUT_FILE=""
MAX_ACCOUNTS=5

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="${2:-}"; shift 2 ;;
    --output-file)
      OUTPUT_FILE="${2:-}"; shift 2 ;;
    --max-accounts)
      MAX_ACCOUNTS="${2:-5}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

if ! [[ "$MAX_ACCOUNTS" =~ ^[0-9]+$ ]] || [[ "$MAX_ACCOUNTS" -lt 1 ]]; then
  echo "--max-accounts 必须是正整数" >&2
  exit 2
fi

if [[ -z "$OUTPUT_FILE" ]]; then
  OUTPUT_FILE="../reports/cloud-mainline-check-${DATE_TAG}.json"
fi
mkdir -p "$(dirname "$OUTPUT_FILE")"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
CLOUD_CHECKS_FILE="$tmp_dir/cloud-checks.jsonl"
: >"$CLOUD_CHECKS_FILE"

base="${BASE_URL%/}"
generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

health_code="$(http_code "${base}/api/health" "$tmp_dir/health.json" || true)"
dashboard_light_code="$(http_code "${base}/api/dashboard?mode=light" "$tmp_dir/dashboard-light.json" || true)"
accounts_code="$(http_code "${base}/api/cloud/accounts" "$tmp_dir/accounts.json" || true)"

account_total=0
enabled_total=0
checked_accounts=0

if [[ "${accounts_code:-0}" == "200" ]]; then
  account_total="$(jq -r '.total // (.items // [] | length)' "$tmp_dir/accounts.json")"
  enabled_total="$(jq -r '[.items // [] | .[] | select(.enabled != false)] | length' "$tmp_dir/accounts.json")"
  checked_accounts="$(
    jq -cr --argjson max "$MAX_ACCOUNTS" '.items // [] | map(select(.enabled != false)) | .[:$max][]' "$tmp_dir/accounts.json" |
    while IFS= read -r account_json; do
      account_id="$(jq -r '.id' <<<"$account_json")"
      account_name="$(jq -r '.name // ""' <<<"$account_json")"
      provider="$(jq -r '.provider // "aliyun"' <<<"$account_json")"
      account_id_q="$(url_encode "$account_id")"

      case "$provider" in
        aliyun)
          ecs_file="$tmp_dir/aliyun-${account_id}-ecs.json"
          ecs_url="${base}/api/cloud/aliyun/instances?accountId=${account_id_q}"
          ecs_code="$(http_code "$ecs_url" "$ecs_file" || true)"
          append_cloud_check "$provider" "$account_id" "$account_name" "ecs" "instances" "$ecs_code" "$ecs_file" "" "" "$ecs_url"

          if [[ "$ecs_code" == "200" ]] && [[ "$(jq -r '.items // [] | length' "$ecs_file")" -gt 0 ]]; then
            instance_id="$(jq -r '.items[0].id // ""' "$ecs_file")"
            region="$(jq -r '.items[0].regionId // ""' "$ecs_file")"
            public_ip="$(jq -r '.items[0].eipAddress // (.items[0].publicIps[0] // "")' "$ecs_file")"
            overview_file="$tmp_dir/aliyun-${account_id}-ecs-overview.json"
            overview_url="${base}/api/cloud/aliyun/overview?accountId=${account_id_q}&instanceId=$(url_encode "$instance_id")&region=$(url_encode "$region")&minutes=30&publicIp=$(url_encode "$public_ip")"
            overview_code="$(http_code "$overview_url" "$overview_file" || true)"
            append_cloud_check "$provider" "$account_id" "$account_name" "ecs" "overview" "$overview_code" "$overview_file" "$region" "$instance_id" "$overview_url"
          fi

          rds_file="$tmp_dir/aliyun-${account_id}-rds.json"
          rds_url="${base}/api/cloud/aliyun/rds/instances?accountId=${account_id_q}"
          rds_code="$(http_code "$rds_url" "$rds_file" || true)"
          append_cloud_check "$provider" "$account_id" "$account_name" "rds" "instances" "$rds_code" "$rds_file" "" "" "$rds_url"

          if [[ "$rds_code" == "200" ]] && [[ "$(jq -r '.items // [] | length' "$rds_file")" -gt 0 ]]; then
            db_id="$(jq -r '.items[0].id // ""' "$rds_file")"
            db_region="$(jq -r '.items[0].regionId // ""' "$rds_file")"
            engine="$(jq -r '.items[0].engine // ""' "$rds_file")"
            rds_overview_file="$tmp_dir/aliyun-${account_id}-rds-overview.json"
            rds_overview_url="${base}/api/cloud/aliyun/rds/overview?accountId=${account_id_q}&dbInstanceId=$(url_encode "$db_id")&region=$(url_encode "$db_region")&engine=$(url_encode "$engine")&minutes=30"
            rds_overview_code="$(http_code "$rds_overview_url" "$rds_overview_file" || true)"
            append_cloud_check "$provider" "$account_id" "$account_name" "rds" "overview" "$rds_overview_code" "$rds_overview_file" "$db_region" "$db_id" "$rds_overview_url"
          fi
          ;;
        huawei)
          ecs_file="$tmp_dir/huawei-${account_id}-ecs.json"
          ecs_url="${base}/api/cloud/huawei/instances?accountId=${account_id_q}"
          ecs_code="$(http_code "$ecs_url" "$ecs_file" || true)"
          append_cloud_check "$provider" "$account_id" "$account_name" "ecs" "instances" "$ecs_code" "$ecs_file" "" "" "$ecs_url"

          if [[ "$ecs_code" == "200" ]] && [[ "$(jq -r '.items // [] | length' "$ecs_file")" -gt 0 ]]; then
            instance_id="$(jq -r '.items[0].id // ""' "$ecs_file")"
            region="$(jq -r '.items[0].regionId // ""' "$ecs_file")"
            overview_file="$tmp_dir/huawei-${account_id}-ecs-overview.json"
            overview_url="${base}/api/cloud/huawei/overview?accountId=${account_id_q}&instanceId=$(url_encode "$instance_id")&region=$(url_encode "$region")&minutes=30"
            overview_code="$(http_code "$overview_url" "$overview_file" || true)"
            append_cloud_check "$provider" "$account_id" "$account_name" "ecs" "overview" "$overview_code" "$overview_file" "$region" "$instance_id" "$overview_url"
          fi

          rds_file="$tmp_dir/huawei-${account_id}-rds.json"
          rds_url="${base}/api/cloud/huawei/rds/instances?accountId=${account_id_q}"
          rds_code="$(http_code "$rds_url" "$rds_file" || true)"
          append_cloud_check "$provider" "$account_id" "$account_name" "rds" "instances" "$rds_code" "$rds_file" "" "" "$rds_url"

          if [[ "$rds_code" == "200" ]] && [[ "$(jq -r '.items // [] | length' "$rds_file")" -gt 0 ]]; then
            db_id="$(jq -r '.items[0].id // ""' "$rds_file")"
            node_id="$(jq -r '.items[0].nodeId // ""' "$rds_file")"
            db_region="$(jq -r '.items[0].regionId // ""' "$rds_file")"
            engine="$(jq -r '.items[0].engine // ""' "$rds_file")"
            rds_overview_file="$tmp_dir/huawei-${account_id}-rds-overview.json"
            rds_overview_url="${base}/api/cloud/huawei/rds/overview?accountId=${account_id_q}&dbInstanceId=$(url_encode "$db_id")&nodeId=$(url_encode "$node_id")&region=$(url_encode "$db_region")&engine=$(url_encode "$engine")&minutes=30"
            rds_overview_code="$(http_code "$rds_overview_url" "$rds_overview_file" || true)"
            append_cloud_check "$provider" "$account_id" "$account_name" "rds" "overview" "$rds_overview_code" "$rds_overview_file" "$db_region" "$db_id" "$rds_overview_url"
          fi
          ;;
        *)
          unsupported_file="$tmp_dir/unsupported-${account_id}.json"
          jq -n --arg error "不支持的云平台: ${provider}" '{error:$error}' >"$unsupported_file"
          append_cloud_check "$provider" "$account_id" "$account_name" "unknown" "instances" 0 "$unsupported_file" "" "" ""
          ;;
      esac
    done
    jq -s 'map(.accountId) | unique | length' "$CLOUD_CHECKS_FILE"
  )"
fi

cloud_failed="$(jq -s '[.[] | select(.ok != true)] | length' "$CLOUD_CHECKS_FILE")"
cloud_total="$(jq -s 'length' "$CLOUD_CHECKS_FILE")"

jq -n \
  --arg generatedAt "$generated_at" \
  --arg baseUrl "$BASE_URL" \
  --argjson healthCode "${health_code:-0}" \
  --argjson dashboardLightCode "${dashboard_light_code:-0}" \
  --argjson accountsCode "${accounts_code:-0}" \
  --argjson accountTotal "${account_total:-0}" \
  --argjson enabledTotal "${enabled_total:-0}" \
  --argjson checkedAccounts "${checked_accounts:-0}" \
  --argjson maxAccounts "$MAX_ACCOUNTS" \
  --argjson cloudTotal "${cloud_total:-0}" \
  --argjson cloudFailed "${cloud_failed:-0}" \
  --slurpfile cloud "$CLOUD_CHECKS_FILE" \
  '{
    generatedAt: $generatedAt,
    baseUrl: $baseUrl,
    checks: {
      health: {code: $healthCode, pass: ($healthCode == 200)},
      dashboardLight: {code: $dashboardLightCode, pass: ($dashboardLightCode == 200)},
      cloudAccounts: {code: $accountsCode, total: $accountTotal, enabled: $enabledTotal, pass: ($accountsCode == 200)}
    },
    cloudResources: {
      skipped: ($enabledTotal == 0),
      maxAccounts: $maxAccounts,
      checkedAccounts: $checkedAccounts,
      totalChecks: $cloudTotal,
      failedChecks: $cloudFailed,
      checks: $cloud
    },
    pass: (
      ($healthCode == 200) and
      ($dashboardLightCode == 200) and
      ($accountsCode == 200) and
      ($cloudFailed == 0)
    )
  }' >"$OUTPUT_FILE"

echo "主线回归结果: $OUTPUT_FILE"
jq . "$OUTPUT_FILE"

if [[ "$(jq -r '.pass' "$OUTPUT_FILE")" != "true" ]]; then
  exit 3
fi

exit 0

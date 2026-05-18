#!/usr/bin/env bash
set -euo pipefail

# 华为云真实账号回归（macOS/Linux）
# 核心目标：抽样核对华为云 ECS/RDS 字段、CES 指标说明和到期状态。

usage() {
  cat <<'EOF'
用法:
  scripts/ops/huawei-real-account-check.sh \
    --base-url http://localhost:8082 \
    --account-id 1 \
    --output-file ../reports/huawei-real-account-check-2026-05-12.json

参数:
  --base-url URL       API 地址，默认 http://localhost:8082
  --account-id ID      指定华为云账号 ID；不传时使用第一个已启用华为云账号
  --output-file FILE   输出文件，默认 ../reports/huawei-real-account-check-YYYY-MM-DD.json
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

require_cmd curl
require_cmd jq

BASE_URL="http://localhost:8082"
ACCOUNT_ID=""
DATE_TAG="$(date +%F)"
OUTPUT_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="${2:-}"; shift 2 ;;
    --account-id)
      ACCOUNT_ID="${2:-}"; shift 2 ;;
    --output-file)
      OUTPUT_FILE="${2:-}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

if [[ -z "$OUTPUT_FILE" ]]; then
  OUTPUT_FILE="../reports/huawei-real-account-check-${DATE_TAG}.json"
fi
mkdir -p "$(dirname "$OUTPUT_FILE")"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

base="${BASE_URL%/}"
generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

accounts_code="$(http_code "${base}/api/cloud/accounts" "$tmp_dir/accounts.json" || true)"
if [[ "${accounts_code:-0}" != "200" ]]; then
  jq -n \
    --arg generatedAt "$generated_at" \
    --arg baseUrl "$BASE_URL" \
    --argjson accountsCode "${accounts_code:-0}" \
    --slurpfile accounts "$tmp_dir/accounts.json" \
    '{
      generatedAt: $generatedAt,
      baseUrl: $baseUrl,
      pass: false,
      reason: "云账号列表接口不可用",
      accountsCode: $accountsCode,
      response: ($accounts[0] // {})
    }' >"$OUTPUT_FILE"
  jq . "$OUTPUT_FILE"
  exit 3
fi

if [[ -z "$ACCOUNT_ID" ]]; then
  ACCOUNT_ID="$(jq -r '.items // [] | map(select(.provider == "huawei" and .enabled != false)) | .[0].id // ""' "$tmp_dir/accounts.json")"
fi

if [[ -z "$ACCOUNT_ID" || "$ACCOUNT_ID" == "null" ]]; then
  jq -n \
    --arg generatedAt "$generated_at" \
    --arg baseUrl "$BASE_URL" \
    --argjson accountsCode "${accounts_code:-0}" \
    --slurpfile accounts "$tmp_dir/accounts.json" \
    '{
      generatedAt: $generatedAt,
      baseUrl: $baseUrl,
      pass: false,
      reason: "未找到已启用的华为云账号，请先在控制台新增或启用账号",
      accountsCode: $accountsCode,
      accounts: ($accounts[0].items // [])
    }' >"$OUTPUT_FILE"
  jq . "$OUTPUT_FILE"
  exit 3
fi

account_id_q="$(url_encode "$ACCOUNT_ID")"
account_json="$(jq -c --arg id "$ACCOUNT_ID" '.items // [] | map(select((.id|tostring) == $id)) | .[0] // {}' "$tmp_dir/accounts.json")"

instances_url="${base}/api/cloud/huawei/instances?accountId=${account_id_q}"
instances_code="$(http_code "$instances_url" "$tmp_dir/instances.json" || true)"

instance_count=0
overview_code=0
metrics_count=0
metric_error_count=0
instance_json="{}"
rds_code=0
rds_count=0
rds_overview_code=0
rds_metrics_count=0
rds_metric_error_count=0
rds_json="{}"
rds_url="${base}/api/cloud/huawei/rds/instances?accountId=${account_id_q}"

if [[ "${instances_code:-0}" == "200" ]]; then
  instance_count="$(jq -r '.items // [] | length' "$tmp_dir/instances.json")"
  if [[ "$instance_count" -gt 0 ]]; then
    instance_json="$(jq -c '.items[0]' "$tmp_dir/instances.json")"
    instance_id="$(jq -r '.id // ""' <<<"$instance_json")"
    region="$(jq -r '.regionId // ""' <<<"$instance_json")"
    overview_url="${base}/api/cloud/huawei/overview?accountId=${account_id_q}&instanceId=$(url_encode "$instance_id")&region=$(url_encode "$region")&minutes=30"
    overview_code="$(http_code "$overview_url" "$tmp_dir/overview.json" || true)"
    if [[ "$overview_code" == "200" ]]; then
      metrics_count="$(jq -r '[.metrics // {} | to_entries[] | select((.value.points // []) | length > 0)] | length' "$tmp_dir/overview.json")"
      metric_error_count="$(jq -r '.errors // {} | length' "$tmp_dir/overview.json")"
    else
      jq -n '{}' >"$tmp_dir/overview.json"
    fi
  else
    jq -n '{}' >"$tmp_dir/overview.json"
  fi
else
  jq -n '{}' >"$tmp_dir/overview.json"
fi

rds_code="$(http_code "$rds_url" "$tmp_dir/rds.json" || true)"
if [[ "${rds_code:-0}" == "200" ]]; then
  rds_count="$(jq -r '.items // [] | length' "$tmp_dir/rds.json")"
  if [[ "$rds_count" -gt 0 ]]; then
    rds_json="$(jq -c '.items[0]' "$tmp_dir/rds.json")"
    db_id="$(jq -r '.id // ""' <<<"$rds_json")"
    node_id="$(jq -r '.nodeId // ""' <<<"$rds_json")"
    db_region="$(jq -r '.regionId // ""' <<<"$rds_json")"
    engine="$(jq -r '.engine // ""' <<<"$rds_json")"
    rds_overview_url="${base}/api/cloud/huawei/rds/overview?accountId=${account_id_q}&dbInstanceId=$(url_encode "$db_id")&nodeId=$(url_encode "$node_id")&region=$(url_encode "$db_region")&engine=$(url_encode "$engine")&minutes=30"
    rds_overview_code="$(http_code "$rds_overview_url" "$tmp_dir/rds-overview.json" || true)"
    if [[ "$rds_overview_code" == "200" ]]; then
      rds_metrics_count="$(jq -r '[.metrics // {} | to_entries[] | select((.value.points // []) | length > 0)] | length' "$tmp_dir/rds-overview.json")"
      rds_metric_error_count="$(jq -r '.errors // {} | length' "$tmp_dir/rds-overview.json")"
    else
      jq -n '{}' >"$tmp_dir/rds-overview.json"
    fi
  else
    jq -n '{}' >"$tmp_dir/rds-overview.json"
  fi
else
  jq -n '{}' >"$tmp_dir/rds-overview.json"
fi

jq -n \
  --arg generatedAt "$generated_at" \
  --arg baseUrl "$BASE_URL" \
  --arg accountId "$ACCOUNT_ID" \
  --arg instancesUrl "$instances_url" \
  --arg rdsUrl "$rds_url" \
  --argjson accountsCode "${accounts_code:-0}" \
  --argjson instancesCode "${instances_code:-0}" \
  --argjson overviewCode "${overview_code:-0}" \
  --argjson rdsCode "${rds_code:-0}" \
  --argjson rdsOverviewCode "${rds_overview_code:-0}" \
  --argjson instanceCount "${instance_count:-0}" \
  --argjson metricsCount "${metrics_count:-0}" \
  --argjson metricErrorCount "${metric_error_count:-0}" \
  --argjson rdsCount "${rds_count:-0}" \
  --argjson rdsMetricsCount "${rds_metrics_count:-0}" \
  --argjson rdsMetricErrorCount "${rds_metric_error_count:-0}" \
  --argjson account "$account_json" \
  --argjson instance "$instance_json" \
  --argjson rds "$rds_json" \
  --slurpfile instances "$tmp_dir/instances.json" \
  --slurpfile overview "$tmp_dir/overview.json" \
  --slurpfile rdsOverview "$tmp_dir/rds-overview.json" \
  '{
    generatedAt: $generatedAt,
    baseUrl: $baseUrl,
    accountId: $accountId,
    account: {
      id: $account.id,
      name: $account.name,
      provider: $account.provider,
      regions: ($account.regions // []),
      metricPeriod: ($account.metricPeriod // "")
    },
    checks: {
      accounts: {code: $accountsCode, pass: ($accountsCode == 200)},
      instances: {code: $instancesCode, url: $instancesUrl, count: $instanceCount, pass: ($instancesCode == 200)},
      overview: {code: $overviewCode, pass: (($instanceCount == 0) or ($overviewCode == 200))},
      rdsInstances: {code: $rdsCode, url: $rdsUrl, count: $rdsCount, pass: ($rdsCode == 200)},
      rdsOverview: {code: $rdsOverviewCode, pass: (($rdsCount == 0) or ($rdsOverviewCode == 200))}
    },
    sample: {
      instance: {
        id: ($instance.id // ""),
        name: ($instance.name // ""),
        regionId: ($instance.regionId // ""),
        zoneId: ($instance.zoneId // ""),
        status: ($instance.status // ""),
        cpu: ($instance.cpu // null),
        memoryMb: ($instance.memoryMb // null),
        publicIps: ($instance.publicIps // []),
        privateIps: ($instance.privateIps // []),
        chargeType: ($instance.chargeType // ""),
        expiredAt: ($instance.expiredAt // ""),
        expirationStatus: ($instance.expirationStatus // ""),
        expirationMessage: ($instance.expirationMessage // "")
      },
      metrics: {
        availableMetricCount: ($overview[0].availableMetricCount // 0),
        nonEmptyMetricGroups: $metricsCount,
        errorMetricGroups: $metricErrorCount,
        units: (
          $overview[0].metrics // {}
          | to_entries
          | map({key: .key, namespace: (.value.namespace // ""), metricName: (.value.metricName // ""), unit: (.value.unit // ""), period: (.value.period // ""), points: ((.value.points // []) | length)})
        ),
        errors: ($overview[0].errors // {})
      },
      rds: {
        id: ($rds.id // ""),
        name: ($rds.name // ""),
        nodeId: ($rds.nodeId // ""),
        engine: ($rds.engine // ""),
        regionId: ($rds.regionId // ""),
        zoneId: ($rds.zoneId // ""),
        status: ($rds.status // ""),
        storageGb: ($rds.storageGb // null),
        connectionString: ($rds.connectionString // ""),
        expirationStatus: ($rds.expirationStatus // ""),
        expirationMessage: ($rds.expirationMessage // ""),
        metrics: {
          availableMetricCount: ($rdsOverview[0].availableMetricCount // 0),
          nonEmptyMetricGroups: $rdsMetricsCount,
          errorMetricGroups: $rdsMetricErrorCount,
          units: (
            $rdsOverview[0].metrics // {}
            | to_entries
            | map({key: .key, namespace: (.value.namespace // ""), metricName: (.value.metricName // ""), unit: (.value.unit // ""), period: (.value.period // ""), points: ((.value.points // []) | length)})
          ),
          errors: ($rdsOverview[0].errors // {})
        }
      }
    },
    pass: (
      ($accountsCode == 200) and
      ($instancesCode == 200) and
      (($instanceCount == 0) or ($overviewCode == 200)) and
      ($rdsCode == 200) and
      (($rdsCount == 0) or ($rdsOverviewCode == 200))
    ),
    notes: [
      (if $instanceCount == 0 then "账号可访问但未返回 ECS 实例，无法核对 CES 指标和到期字段" else empty end),
      (if ($metricsCount == 0 and $instanceCount > 0) then "已返回实例但暂无 CES 采样点，请检查 CES 权限、实例地域、Agent 和采样延迟" else empty end),
      (if $rdsCount == 0 then "账号可访问但未返回 RDS 实例，无法核对 RDS 监控字段" else empty end),
      (if ($rdsMetricsCount == 0 and $rdsCount > 0) then "已返回 RDS 实例但暂无 CES 采样点，请检查 CES 权限、实例地域和采样延迟" else empty end)
    ]
  }' >"$OUTPUT_FILE"

echo "华为云真实账号回归结果: $OUTPUT_FILE"
jq . "$OUTPUT_FILE"

if [[ "$(jq -r '.pass' "$OUTPUT_FILE")" != "true" ]]; then
  exit 3
fi

exit 0

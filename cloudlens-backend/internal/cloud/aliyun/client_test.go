package aliyun

import (
	"testing"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/rds"
)

func TestResolveExpirationForPrepaidInstance(t *testing.T) {
	now := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)

	info := resolveExpiration("2026-05-21T09:59:00Z", "PrePaid", false, now)
	if info.Status != expirationStatusExpiring {
		t.Fatalf("期望 30 天内到期状态为 %s，实际 %s", expirationStatusExpiring, info.Status)
	}
	if info.ExpiresInDays == nil || *info.ExpiresInDays != 9 {
		t.Fatalf("期望剩余 9 天，实际 %#v", info.ExpiresInDays)
	}
	if info.Message != "剩余 9 天" {
		t.Fatalf("期望消息为剩余 9 天，实际 %s", info.Message)
	}
}

func TestResolveExpirationForExpiredInstance(t *testing.T) {
	now := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)

	info := resolveExpiration("2026-05-11T09:59:00Z", "PrePaid", false, now)
	if info.Status != expirationStatusExpired {
		t.Fatalf("期望已到期状态为 %s，实际 %s", expirationStatusExpired, info.Status)
	}
	if info.ExpiresInDays == nil || *info.ExpiresInDays != 0 {
		t.Fatalf("期望已到期剩余天数为 0，实际 %#v", info.ExpiresInDays)
	}
}

func TestResolveExpirationForPostPaidInstance(t *testing.T) {
	info := resolveExpiration("2099-12-31T23:59:59Z", "PostPaid", false, time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	if info.Status != expirationStatusNoExpiration {
		t.Fatalf("期望按量付费无固定到期日，实际 %s", info.Status)
	}
	if info.ExpiresInDays != nil {
		t.Fatalf("期望按量付费不返回剩余天数，实际 %#v", info.ExpiresInDays)
	}
}

func TestResolveExpirationTreatsFarFutureAsPlaceholder(t *testing.T) {
	info := resolveExpiration("2099-12-31T23:59:59Z", "", false, time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	if info.Status != expirationStatusNoExpiration {
		t.Fatalf("期望远期占位时间视为无固定到期日，实际 %s", info.Status)
	}
	if info.ExpiresInDays != nil {
		t.Fatalf("期望远期占位时间不返回剩余天数，实际 %#v", info.ExpiresInDays)
	}
	if info.Message != "云厂商返回远期占位时间，视为无固定到期日" {
		t.Fatalf("期望远期占位说明，实际 %s", info.Message)
	}
}

func TestResolveExpirationForSpotInstance(t *testing.T) {
	info := resolveExpiration("2099-12-31T23:59:59Z", "PostPaid", true, time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	if info.Status != expirationStatusNoExpiration {
		t.Fatalf("期望抢占式实例无固定到期日，实际 %s", info.Status)
	}
	if info.ExpiresInDays != nil {
		t.Fatalf("期望抢占式实例不返回剩余天数，实际 %#v", info.ExpiresInDays)
	}
	if info.Message != "抢占式实例按量计费，无固定到期日" {
		t.Fatalf("期望标注抢占式实例，实际 %s", info.Message)
	}
}

func TestIsSpotInstanceFromStrategy(t *testing.T) {
	if !isSpotInstance(false, "SpotAsPriceGo") {
		t.Fatal("期望 SpotAsPriceGo 识别为抢占式实例")
	}
	if isSpotInstance(false, "NoSpot") {
		t.Fatal("期望 NoSpot 不识别为抢占式实例")
	}
}

func TestAliyunMetricUnitDocumentsOverviewUnits(t *testing.T) {
	cases := map[string]string{
		"CPUUtilization":                 "%",
		"memory_usedutilization":         "%",
		"InternetInRate":                 "bit/s",
		"VPC_PublicIP_InternetOutRate":   "bit/s",
		"DiskReadBPS":                    "Byte/s",
		"ecs.UnknownMetricForRegression": "",
	}
	for metricName, expected := range cases {
		if actual := aliyunMetricUnit(metricName); actual != expected {
			t.Fatalf("指标 %s 单位期望 %q，实际 %q", metricName, expected, actual)
		}
	}
}

func TestNewClientNormalizesInvalidMetricPeriod(t *testing.T) {
	client, err := NewClient(Config{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		MetricPeriod:    "not-a-number",
	})
	if err != nil {
		t.Fatalf("创建阿里云客户端失败: %v", err)
	}
	if client.config.MetricPeriod != "60" {
		t.Fatalf("非法采样周期应回退到 60 秒，实际 %s", client.config.MetricPeriod)
	}
}

func TestNormalizeSamplingPeriodKeepsPositiveSeconds(t *testing.T) {
	cases := map[string]string{
		"":      "60",
		"0":     "60",
		"-1":    "60",
		" 300 ": "300",
	}
	for input, expected := range cases {
		if actual := normalizeSamplingPeriod(input); actual != expected {
			t.Fatalf("采样周期 %q 期望 %s，实际 %s", input, expected, actual)
		}
	}
}

func TestTrafficKbitToBitRateUsesSamplingPeriod(t *testing.T) {
	got := trafficKbitToBitRate(120, 60)
	if got != 2000 {
		t.Fatalf("期望 120 Kbit / 60s 换算为 2000 bit/s，实际 %.2f", got)
	}
}

func TestParseAliyunTimeSupportsMinutePrecision(t *testing.T) {
	parsed, ok := parseAliyunTime("2026-05-21T09:59Z")
	if !ok {
		t.Fatal("期望能解析阿里云分钟精度时间")
	}
	if parsed.UTC().Format(time.RFC3339) != "2026-05-21T09:59:00Z" {
		t.Fatalf("解析后的时间不符合预期: %s", parsed.UTC().Format(time.RFC3339))
	}
}

func TestParseMetricPointsSupportsTimestampAliases(t *testing.T) {
	raw := `[
		{"Timestamp": 1778493600000, "Average": "12.5"},
		{"timestamp": 1778493660, "value": 0},
		{"Date": "2026-05-11T10:02:00Z", "Value": 7},
		{"timestamp": 0, "Average": 99},
		{"timestamp": 1778493780000, "NotAValue": 1}
	]`

	points, err := parseMetricPoints(raw)
	if err != nil {
		t.Fatalf("解析云监控采样失败: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("期望保留 3 个有效采样点，实际 %#v", points)
	}
	if points[0].Timestamp != 1778493600000 || points[0].Value != 12.5 {
		t.Fatalf("第一个采样点不符合预期: %#v", points[0])
	}
	if points[1].Timestamp != 1778493660000 || points[1].Value != 0 {
		t.Fatalf("秒级时间戳或 0 值采样处理不符合预期: %#v", points[1])
	}
	if points[2].Timestamp != 1778493720000 || points[2].Value != 7 {
		t.Fatalf("Date 时间采样处理不符合预期: %#v", points[2])
	}
}

func TestRDSPerformanceKeysUseEngineSpecificList(t *testing.T) {
	mysqlKeys := rdsPerformanceKeys("MySQL")
	if !containsString(mysqlKeys, "MySQL_QPSTPS") {
		t.Fatal("期望 MySQL 指标包含 MySQL_QPSTPS")
	}
	if containsString(mysqlKeys, "SQLServer_QPS") {
		t.Fatal("MySQL 指标不应混入 SQLServer_QPS")
	}

	sqlServerKeys := rdsPerformanceKeys("SQLServer")
	if !containsString(sqlServerKeys, "SQLServer_InstanceCPUUsage") {
		t.Fatal("期望 SQLServer 指标包含 SQLServer_InstanceCPUUsage")
	}
}

func TestBuildRDSMetricSeriesSplitsValueFormat(t *testing.T) {
	series := buildRDSMetricSeries([]rds.PerformanceKey{
		{
			Key:         "MySQL_QPSTPS",
			Unit:        "Second",
			ValueFormat: "qps&tps",
			Values: rds.ValuesInDescribeDBInstancePerformance{
				PerformanceValue: []rds.PerformanceValue{
					{Date: "2026-05-11T10:00:00Z", Value: "12.5&3"},
					{Date: "2026-05-11T10:01:00Z", Value: "13.5&4"},
				},
			},
		},
	}, "rm-test", "cn-hangzhou", "MySQL", "60")

	qps := series["MySQL_QPSTPS.qps"]
	if qps == nil {
		t.Fatal("期望拆出 qps 子指标")
	}
	if qps.Unit != "Second" || qps.SubKey != "qps" {
		t.Fatalf("子指标元数据不符合预期: %#v", qps)
	}
	if len(qps.Points) != 2 || qps.Points[0].Value != 12.5 || qps.Points[1].Value != 13.5 {
		t.Fatalf("qps 采样不符合预期: %#v", qps.Points)
	}
	tps := series["MySQL_QPSTPS.tps"]
	if tps == nil || len(tps.Points) != 2 || tps.Points[1].Value != 4 {
		t.Fatalf("tps 采样不符合预期: %#v", tps)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

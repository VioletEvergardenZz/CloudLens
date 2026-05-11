package huawei

import "testing"

func TestHuaweiMetricUnitDocumentsOverviewUnits(t *testing.T) {
	cases := map[string]string{
		"cpu_util":                              "%",
		"mem_usedPercent":                       "%",
		"disk_util_inband":                      "%",
		"network_incoming_bytes_aggregate_rate": "Byte/s",
		"disk_read_bytes_rate":                  "Byte/s",
		"load_average1":                         "",
	}
	for metricName, expected := range cases {
		if actual := huaweiMetricUnit(NamespaceECS, metricName); actual != expected {
			t.Fatalf("指标 %s 单位期望 %q，实际 %q", metricName, expected, actual)
		}
	}
}

func TestResolveMetricSeriesUnitUsesDatapointUnitFirst(t *testing.T) {
	points := []MetricPoint{
		{Timestamp: 1, Value: 10, Raw: map[string]any{"unit": "Byte/s"}},
	}
	if actual := resolveMetricSeriesUnit(points, "bit/s"); actual != "Byte/s" {
		t.Fatalf("期望优先使用 datapoint 单位，实际 %q", actual)
	}
}

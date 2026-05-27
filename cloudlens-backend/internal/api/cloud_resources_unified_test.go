package api

import (
	"encoding/json"
	"testing"
)

func TestNormalizeCloudSnapshotResources(t *testing.T) {
	payload, err := json.Marshal([]cloudUnifiedSnapshotResource{
		{
			ID:               "i-demo-001",
			Name:             "kind-worker",
			Provider:         "aliyun",
			RegionID:         "cn-hangzhou",
			ZoneID:           "cn-hangzhou-h",
			Status:           "Running",
			PrivateIPs:       []string{"172.16.1.10", "172.16.1.10"},
			PublicIPs:        []string{"1.1.1.1"},
			EipAddress:       "2.2.2.2",
			ChargeType:       "PrePaid",
			ExpirationStatus: "expiring",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	items := normalizeCloudSnapshotResources(cloudResourceSnapshot{
		AccountID:     7,
		Provider:      "aliyun",
		ResourceType:  "ecs",
		PayloadJSON:   payload,
		Source:        "live",
		LastSuccessAt: "2026-05-27T00:00:00Z",
	}, cloudAccountRecord{Name: "生产账号"})

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ID != "7:ecs:i-demo-001" {
		t.Fatalf("unexpected id: %s", item.ID)
	}
	if item.AccountName != "生产账号" || item.Region != "cn-hangzhou" || item.Zone != "cn-hangzhou-h" {
		t.Fatalf("unexpected mapped item: %#v", item)
	}
	if len(item.PrivateIPs) != 1 || item.PrivateIPs[0] != "172.16.1.10" {
		t.Fatalf("private ip should be compacted: %#v", item.PrivateIPs)
	}
	if len(item.PublicIPs) != 2 {
		t.Fatalf("public ip and eip should be kept: %#v", item.PublicIPs)
	}
}

func TestCloudUnifiedResourceMatches(t *testing.T) {
	item := cloudUnifiedResource{
		AccountID:    3,
		Provider:     "huawei",
		ResourceType: "rds",
		Region:       "cn-north-4",
	}
	filter := cloudResourceFilter{
		AccountID:    3,
		Provider:     "huawei",
		ResourceType: "rds",
		Region:       "cn-north-4",
	}
	if !cloudUnifiedResourceMatches(item, filter) {
		t.Fatal("resource should match exact filter")
	}
	filter.Region = "cn-east-3"
	if cloudUnifiedResourceMatches(item, filter) {
		t.Fatal("resource should not match another region")
	}
}

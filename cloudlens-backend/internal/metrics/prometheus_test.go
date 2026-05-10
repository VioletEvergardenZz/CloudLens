// 本文件用于 Prometheus 指标测试 保障指标文本格式与核心字段可用

package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

func TestCollectorRenderPrometheus(t *testing.T) {
	collector := NewCollector()
	collector.ResetForTest()

	collector.SetQueueStats(models.UploadStats{
		QueueLength: 7,
		Workers:     3,
		InFlight:    2,
	})
	collector.IncFileEvent()
	collector.IncQueueFull()
	collector.IncQueueShed()
	collector.IncUploadRetry()
	collector.ObserveUploadSuccess(350 * time.Millisecond)
	collector.ObserveUploadFailure("network timeout")
	collector.ObserveAILogSummary("success", 1200*time.Millisecond, 1)
	collector.ObserveKBSearch(2)
	collector.ObserveKBAsk(1)
	collector.ObserveKBReviewLatency(180 * time.Millisecond)

	out := collector.RenderPrometheus()

	mustContain := []string{
		"cloudlens_file_events_total 1",
		"cloudlens_upload_queue_length 7",
		"cloudlens_upload_workers 3",
		"cloudlens_upload_inflight 2",
		"cloudlens_upload_queue_full_total 1",
		"cloudlens_upload_queue_shed_total 1",
		"cloudlens_upload_retry_total 1",
		"cloudlens_upload_success_total 1",
		"cloudlens_upload_failure_total 1",
		`cloudlens_upload_failure_reason_total{reason="network timeout"} 1`,
		`cloudlens_ai_log_summary_total{outcome="success"} 1`,
		"cloudlens_ai_log_summary_retry_total 1",
		"cloudlens_kb_search_total 1",
		"cloudlens_kb_search_hit_total 1",
		"cloudlens_kb_search_hit_ratio 1",
		"cloudlens_kb_ask_total 1",
		"cloudlens_kb_ask_citation_total 1",
		"cloudlens_kb_ask_citation_ratio 1",
	}
	for _, token := range mustContain {
		if !strings.Contains(out, token) {
			t.Fatalf("prometheus output missing token %q\noutput:\n%s", token, out)
		}
	}
}

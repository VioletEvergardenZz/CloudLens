// 本文件用于云资产轻量巡检报告。
// 文件职责：组合诊断、风险、运行检查，并支持 Markdown 渲染。

package api

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

func (h *handler) buildCloudInspectionReport() cloudInspectionReport {
	diagnostics, _, diagErr := h.buildCloudDiagnostics()
	risks, riskSummary, riskErr := h.buildCloudRisks()
	runtimeChecks := h.buildRuntimeChecks()
	if diagErr != nil {
		runtimeChecks = append(runtimeChecks, cloudOpsCheck{
			Name:       "云账号诊断",
			Status:     "error",
			Severity:   "critical",
			Message:    diagErr.Error(),
			Suggestion: "确认 data/cloud 目录可写，或查看后端启动日志中的云账号存储初始化错误",
		})
	}
	if riskErr != nil {
		runtimeChecks = append(runtimeChecks, cloudOpsCheck{
			Name:       "风险中心",
			Status:     "error",
			Severity:   "critical",
			Message:    riskErr.Error(),
			Suggestion: "风险中心依赖资源快照，请先确认云账号存储可用",
		})
	}
	nextActions := buildCloudInspectionNextActions(diagnostics, risks, runtimeChecks)
	summary := map[string]int{
		"accounts":      len(diagnostics),
		"risks":         len(risks),
		"criticalRisk":  riskSummary.Critical,
		"warningRisk":   riskSummary.Warning,
		"runtimeChecks": len(runtimeChecks),
	}
	report := cloudInspectionReport{
		GeneratedAt: formatCloudTime(time.Now().UTC()),
		Status:      summarizeCloudOpsChecks(runtimeChecks),
		Summary:     summary,
		Diagnostics: diagnostics,
		Risks:       risks,
		Runtime:     runtimeChecks,
		NextActions: nextActions,
	}
	if riskSummary.Critical > 0 || hasCloudOpsCheckStatus(runtimeChecks, "error") {
		report.Status = "error"
	} else if riskSummary.Warning > 0 || len(risks) > 0 || report.Status == "warning" {
		report.Status = "warning"
	}
	return report
}

func summarizeCloudOpsChecks(checks []cloudOpsCheck) string {
	status := "ok"
	for _, check := range checks {
		switch check.Status {
		case "error":
			return "error"
		case "warning":
			status = "warning"
		}
	}
	return status
}

func hasCloudOpsCheckStatus(checks []cloudOpsCheck, status string) bool {
	for _, check := range checks {
		if check.Status == status {
			return true
		}
	}
	return false
}

func buildCloudInspectionNextActions(diagnostics []cloudAccountDiagnostic, risks []cloudRiskItem, runtimeChecks []cloudOpsCheck) []string {
	out := make([]string, 0, 5)
	if hasCloudOpsCheckStatus(runtimeChecks, "error") {
		out = append(out, "先处理运行体检中的 error 项，确保本地数据目录、密钥和数据库可用")
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Status == "error" || diagnostic.Status == "warning" {
			out = append(out, fmt.Sprintf("重新测试云账号 %s，并按提示补齐只读权限", diagnostic.Account.Name))
			break
		}
	}
	for _, risk := range risks {
		if risk.Severity == "critical" {
			out = append(out, "优先处理 critical 风险，尤其是已过期资源和 RDS 存储高水位")
			break
		}
	}
	if len(risks) > 0 {
		out = append(out, "将可确认的风险同步到告警处置记录，形成 open -> processing -> recovered 闭环")
	}
	if len(out) == 0 {
		out = append(out, "保持账号测试和资源同步节奏，下一步可接入第二轮 provider 抽象与告警联动")
	}
	return out
}

func renderCloudInspectionMarkdown(report cloudInspectionReport) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# CloudLens 轻量巡检报告\n\n")
	fmt.Fprintf(&buf, "- 生成时间：%s\n", report.GeneratedAt)
	fmt.Fprintf(&buf, "- 总体状态：%s\n", report.Status)
	fmt.Fprintf(&buf, "- 云账号：%d\n", report.Summary["accounts"])
	fmt.Fprintf(&buf, "- 风险项：%d（critical %d / warning %d）\n\n", report.Summary["risks"], report.Summary["criticalRisk"], report.Summary["warningRisk"])
	fmt.Fprintf(&buf, "## 下一步动作\n\n")
	for _, action := range report.NextActions {
		fmt.Fprintf(&buf, "- %s\n", action)
	}
	fmt.Fprintf(&buf, "\n## 运行体检\n\n")
	for _, check := range report.Runtime {
		fmt.Fprintf(&buf, "- **%s**：%s，%s\n", check.Name, check.Status, check.Message)
	}
	fmt.Fprintf(&buf, "\n## 云账号诊断\n\n")
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintf(&buf, "- **%s**：%s，快照 %d 组\n", diagnostic.Account.Name, diagnostic.Status, len(diagnostic.Snapshots))
	}
	fmt.Fprintf(&buf, "\n## 风险明细\n\n")
	if len(report.Risks) == 0 {
		fmt.Fprintf(&buf, "- 暂无风险项\n")
		return buf.String()
	}
	for _, risk := range report.Risks {
		fmt.Fprintf(&buf, "- [%s] %s / %s：%s\n", risk.Severity, risk.AccountName, risk.ResourceName, risk.Message)
	}
	return buf.String()
}

func fallbackCloudMessage(message, fallback string) string {
	if strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	return fallback
}

// 本文件用于覆盖告警输入源解析与 Docker 选择器匹配

package alert

import "testing"

func TestNormalizeLogSources(t *testing.T) {
	raw := " /var/log/app/error.log , docker://service/api?project=demo docker://backend "
	sources, err := normalizeLogSources(raw)
	if err != nil {
		t.Fatalf("normalizeLogSources failed: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("unexpected source count: got=%d want=3", len(sources))
	}
	if sources[0] != "/var/log/app/error.log" {
		t.Fatalf("unexpected file source: %s", sources[0])
	}
	if sources[1] != "docker://service/api?project=demo" {
		t.Fatalf("unexpected docker service source: %s", sources[1])
	}
	if sources[2] != "docker://container/backend" {
		t.Fatalf("unexpected docker container shorthand: %s", sources[2])
	}
}

func TestSplitAlertLogSources(t *testing.T) {
	raw := []string{"/var/log/app/error.log", "docker://service/api?project=demo", "docker://container/backend"}
	filePaths, dockerSources, err := splitAlertLogSources(raw)
	if err != nil {
		t.Fatalf("splitAlertLogSources failed: %v", err)
	}
	if len(filePaths) != 1 || filePaths[0] != "/var/log/app/error.log" {
		t.Fatalf("unexpected file paths: %#v", filePaths)
	}
	if len(dockerSources) != 2 {
		t.Fatalf("unexpected docker source count: %d", len(dockerSources))
	}
	if dockerSources[0].Kind != logSourceDockerService || dockerSources[0].Selector != "api" || dockerSources[0].Project != "demo" {
		t.Fatalf("unexpected docker service source: %#v", dockerSources[0])
	}
	if dockerSources[1].Kind != logSourceDockerContainer || dockerSources[1].Selector != "backend" {
		t.Fatalf("unexpected docker container source: %#v", dockerSources[1])
	}
}

func TestMatchDockerSource(t *testing.T) {
	container := dockerContainerInfo{
		ID:    "abcdef1234567890",
		Names: []string{"/demo-api-1"},
		Labels: map[string]string{
			"com.docker.compose.project": "demo",
			"com.docker.compose.service": "api",
		},
	}
	if !matchDockerSource(logSource{Kind: logSourceDockerContainer, Selector: "demo-api-1"}, container) {
		t.Fatal("expected container selector match by name")
	}
	if !matchDockerSource(logSource{Kind: logSourceDockerContainer, Selector: "abcdef123456"}, container) {
		t.Fatal("expected container selector match by id prefix")
	}
	if !matchDockerSource(logSource{Kind: logSourceDockerService, Selector: "api", Project: "demo"}, container) {
		t.Fatal("expected compose service selector match")
	}
	if matchDockerSource(logSource{Kind: logSourceDockerService, Selector: "worker", Project: "demo"}, container) {
		t.Fatal("unexpected match for other service")
	}
}

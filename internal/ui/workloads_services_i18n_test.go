package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderServicesList bilingual lockdown: the English output must stay
// byte-identical to the previously hardcoded strings (golden), and the
// Chinese output must switch (except the "(Total: N)" shell, which the
// sibling workloads sections keep untranslated by design).
func TestRenderServicesListEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.clusterData = &model.ClusterData{
		Services: []*model.ServiceData{
			{
				Name: "svc-a", Namespace: "default", Type: "ClusterIP",
				ClusterIP: "10.0.0.1", EndpointCount: 2,
				Ports: []model.ServicePort{{Name: "http", Protocol: "TCP", Port: 80}},
			},
			{
				Name: "web", Namespace: "prod", Type: "NodePort",
				ClusterIP: "10.0.0.2", EndpointCount: 0,
				Ports: []model.ServicePort{
					{Name: "http", Protocol: "TCP", Port: 80, NodePort: 30080},
					{Name: "https", Protocol: "TCP", Port: 443, NodePort: 30443},
					{Name: "metrics", Protocol: "TCP", Port: 8080, NodePort: 30081},
				},
			},
		},
	}
	en.selectedIndex = 1

	rows, count := en.renderServicesList(0)
	if count != 2 {
		t.Errorf("renderServicesList count = %d, want 2", count)
	}

	want := []string{
		"Services  (Total: 2)",
		"",
		"NAME" + strings.Repeat(" ", 24) + "  " +
			"NAMESPACE" + strings.Repeat(" ", 3) + "  " +
			"TYPE" + strings.Repeat(" ", 10) + "  " +
			"CLUSTER-IP" + strings.Repeat(" ", 6) + "  " +
			"PORTS" + strings.Repeat(" ", 19) + "  " +
			"ENDPOINTS" + strings.Repeat(" ", 1),
		"  " + "svc-a" + strings.Repeat(" ", 23) + "  " +
			"default" + strings.Repeat(" ", 5) + "  " +
			"ClusterIP" + strings.Repeat(" ", 5) + "  " +
			"10.0.0.1" + strings.Repeat(" ", 8) + "  " +
			"80/TCP" + strings.Repeat(" ", 18) + "  " +
			"2" + strings.Repeat(" ", 9),
		"> " + "web" + strings.Repeat(" ", 25) + "  " +
			"prod" + strings.Repeat(" ", 8) + "  " +
			"NodePort" + strings.Repeat(" ", 6) + "  " +
			"10.0.0.2" + strings.Repeat(" ", 8) + "  " +
			// First 2 ports, NodePort flavour, then truncated to 24 columns.
			"80:30080/TCP,443:3044..." + "  " +
			"0" + strings.Repeat(" ", 9),
	}
	if len(rows) != len(want) {
		t.Fatalf("renderServicesList returned %d rows, want %d:\n%s", len(rows), len(want), strings.Join(rows, "\n"))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}

	// Empty list: same header shell, empty state with its two-space indent.
	en.clusterData = &model.ClusterData{}
	rows, count = en.renderServicesList(0)
	if count != 0 {
		t.Errorf("empty renderServicesList count = %d, want 0", count)
	}
	wantEmpty := []string{
		"Services  (Total: 0)",
		"",
		want[2],
		"  No services found",
	}
	if len(rows) != len(wantEmpty) {
		t.Fatalf("empty renderServicesList returned %d rows, want %d:\n%s", len(rows), len(wantEmpty), strings.Join(rows, "\n"))
	}
	for i := range wantEmpty {
		if rows[i] != wantEmpty[i] {
			t.Errorf("empty row %d = %q, want %q", i, rows[i], wantEmpty[i])
		}
	}
}

func TestRenderServicesListZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	zh.clusterData = &model.ClusterData{
		Services: []*model.ServiceData{
			{
				Name: "svc-a", Namespace: "default", Type: "ClusterIP",
				ClusterIP: "10.0.0.1", EndpointCount: 2,
				Ports: []model.ServicePort{{Name: "http", Protocol: "TCP", Port: 80}},
			},
		},
	}

	rows, _ := zh.renderServicesList(0)
	joined := strings.Join(rows, "\n")
	for _, noun := range []string{"服务", "名称", "命名空间", "类型", "集群 IP", "端口", "端点"} {
		if !strings.Contains(joined, noun) {
			t.Errorf("zh services list %q does not contain %q", joined, noun)
		}
	}
	for _, bad := range []string{
		"Services", "NAME", "NAMESPACE", "TYPE", "CLUSTER-IP", "PORTS", "ENDPOINTS",
		"No services found", "{{", "}}", "<no value", "%!", "workloads.", "columns.",
	} {
		if strings.Contains(joined, bad) {
			t.Errorf("zh services list %q contains %q", joined, bad)
		}
	}

	// Empty state switches, keeping the two-space indent.
	zh.clusterData = &model.ClusterData{}
	rows, _ = zh.renderServicesList(0)
	joined = strings.Join(rows, "\n")
	if !strings.Contains(joined, "  未找到服务") {
		t.Errorf("zh empty services list %q does not contain 未找到服务", joined)
	}
	if strings.Contains(joined, "No services found") {
		t.Errorf("zh empty services list %q still contains the English empty state", joined)
	}
}

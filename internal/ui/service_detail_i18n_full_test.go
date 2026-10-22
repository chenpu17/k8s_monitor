package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderServiceBasicInfo / renderServicePorts / renderServiceSelector /
// renderServicePods / renderServiceDetail bilingual lockdown: the English
// output must stay byte-identical to the previously hardcoded strings
// (including leading indentation), and the Chinese output must switch. The
// endpoints block is already locked down by service_detail_i18n_test.go
// (batch 11). forcedAsciiProfile keeps the goldens plain text.

func TestRenderServiceBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	// Golden pins "5m", which holds while the fixture-to-render delay stays
	// under 60s — the -5m offset sits exactly at the lower edge of the
	// [5m,6m) window, so any real execution delay only moves deeper into it.
	created := time.Now().Add(-5 * time.Minute)

	minimal := &model.ServiceData{Name: "svc-x", Namespace: "default", Type: "NodePort", CreationTimestamp: created}
	wantMinimal := " 🔌 Service: svc-x \n\n" +
		"  Namespace: default\n" +
		"  Type: NodePort\n" +
		"  Age: 5m"
	if got := en.renderServiceBasicInfo(minimal); got != wantMinimal {
		t.Errorf("en minimal basic info:\n got %q\nwant %q", got, wantMinimal)
	}

	full := &model.ServiceData{
		Name: "svc-x", Namespace: "default", Type: "NodePort", CreationTimestamp: created,
		ClusterIP:      "10.0.0.1",
		ExternalIPs:    []string{"203.0.113.7", "203.0.113.9"},
		LoadBalancerIP: "198.51.100.3",
		Ingress:        []string{"bookinfo.example.com"},
	}
	wantFull := " 🔌 Service: svc-x \n\n" +
		"  Namespace: default\n" +
		"  Type: NodePort\n" +
		"  Cluster IP: 10.0.0.1\n" +
		"  External IPs: 203.0.113.7, 203.0.113.9\n" +
		"  LoadBalancer IP: 198.51.100.3\n" +
		"  Ingress: bookinfo.example.com\n" +
		"  Age: 5m"
	if got := en.renderServiceBasicInfo(full); got != wantFull {
		t.Errorf("en full basic info:\n got %q\nwant %q", got, wantFull)
	}
}

func TestRenderServiceBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	// Golden pins "5m", which holds while the fixture-to-render delay stays
	// under 60s — the -5m offset sits exactly at the lower edge of the
	// [5m,6m) window, so any real execution delay only moves deeper into it.
	created := time.Now().Add(-5 * time.Minute)

	full := &model.ServiceData{
		Name: "svc-x", Namespace: "default", Type: "NodePort", CreationTimestamp: created,
		ClusterIP:      "10.0.0.1",
		ExternalIPs:    []string{"203.0.113.7", "203.0.113.9"},
		LoadBalancerIP: "198.51.100.3",
		Ingress:        []string{"bookinfo.example.com"},
	}
	got := zh.renderServiceBasicInfo(full)
	for _, noun := range []string{"🔌 服务：svc-x", "命名空间", "类型", "集群 IP", "外部 IP", "负载均衡 IP", "Ingress", "年龄"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh basic info %q does not contain %q", got, noun)
		}
	}
	// "Ingress" is kept untranslated on purpose; everything else must switch.
	for _, bad := range []string{
		"Service:", "Namespace", "Type:", "Cluster IP", "External IPs",
		"LoadBalancer IP", "Age:", "{{", "}}", "<no value", "%!", "service.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

func TestRenderServicePortsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	empty := en.renderServicePorts(&model.ServiceData{Name: "svc-x", Namespace: "default"})
	wantEmpty := "Ports\n\n  No ports configured"
	if empty != wantEmpty {
		t.Errorf("en zero-ports render:\n got %q\nwant %q", empty, wantEmpty)
	}

	ports := &model.ServiceData{Name: "svc-x", Namespace: "default", Ports: []model.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, TargetPort: "web", NodePort: 30080},
		{Name: "", Protocol: "UDP", Port: 53, TargetPort: "dns", NodePort: 0},
	}}
	got := en.renderServicePorts(ports)
	want := "Ports\n\n" +
		"  " + "NAME" + strings.Repeat(" ", 16) + "  " +
		"PROTOCOL" + strings.Repeat(" ", 2) + "  " +
		"PORT" + strings.Repeat(" ", 6) + "  " +
		"TARGET PORT" + strings.Repeat(" ", 4) + "  " +
		"NODE PORT" + strings.Repeat(" ", 3) + "\n" +
		"  " + "http" + strings.Repeat(" ", 16) + "  " +
		"TCP" + strings.Repeat(" ", 7) + "  " +
		"80" + strings.Repeat(" ", 8) + "  " +
		"web" + strings.Repeat(" ", 12) + "  " +
		"30080" + strings.Repeat(" ", 7) + "\n" +
		"  " + "-" + strings.Repeat(" ", 19) + "  " +
		"UDP" + strings.Repeat(" ", 7) + "  " +
		"53" + strings.Repeat(" ", 8) + "  " +
		"dns" + strings.Repeat(" ", 12) + "  " +
		"-" + strings.Repeat(" ", 11)
	if got != want {
		t.Errorf("en ports table:\n got %q\nwant %q", got, want)
	}
}

func TestRenderServicePortsZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	empty := zh.renderServicePorts(&model.ServiceData{Name: "svc-x", Namespace: "default"})
	for _, noun := range []string{"端口", "未配置端口"} {
		if !strings.Contains(empty, noun) {
			t.Errorf("zh zero-ports render %q does not contain %q", empty, noun)
		}
	}
	for _, bad := range []string{"No ports configured", "Ports", "{{", "}}", "<no value", "%!", "service."} {
		if strings.Contains(empty, bad) {
			t.Errorf("zh zero-ports render %q contains %q", empty, bad)
		}
	}

	ports := &model.ServiceData{Name: "svc-x", Namespace: "default", Ports: []model.ServicePort{
		{Name: "http", Protocol: "TCP", Port: 80, TargetPort: "web", NodePort: 30080},
	}}
	got := zh.renderServicePorts(ports)
	for _, noun := range []string{"端口", "名称", "协议", "目标端口", "节点端口"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh ports table %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"NAME", "PROTOCOL", "TARGET PORT", "NODE PORT", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh ports table %q contains %q", got, bad)
		}
	}
}

func TestRenderServiceSelectorEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	empty := en.renderServiceSelector(&model.ServiceData{Name: "svc-x", Namespace: "default"})
	wantEmpty := "Selector\n\n  No selector configured\n\nLabels\n\n  No labels"
	if empty != wantEmpty {
		t.Errorf("en zero-selector render:\n got %q\nwant %q", empty, wantEmpty)
	}

	full := &model.ServiceData{
		Name: "svc-x", Namespace: "default",
		Selector: map[string]string{"app": "web", "tier": "frontend"},
		Labels:   map[string]string{"env": "prod", "team": "api"},
	}
	got := en.renderServiceSelector(full)
	want := "Selector\n\n" +
		"  app: web\n" +
		"  tier: frontend\n" +
		"\n" +
		"Labels\n\n" +
		"  env: prod\n" +
		"  team: api"
	if got != want {
		t.Errorf("en selector render:\n got %q\nwant %q", got, want)
	}
}

func TestRenderServiceSelectorZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	empty := zh.renderServiceSelector(&model.ServiceData{Name: "svc-x", Namespace: "default"})
	for _, noun := range []string{"选择器", "未配置选择器", "标签", "无标签"} {
		if !strings.Contains(empty, noun) {
			t.Errorf("zh zero-selector render %q does not contain %q", empty, noun)
		}
	}
	for _, bad := range []string{"Selector", "Labels", "No selector configured", "No labels", "{{", "}}", "<no value", "%!", "service."} {
		if strings.Contains(empty, bad) {
			t.Errorf("zh zero-selector render %q contains %q", empty, bad)
		}
	}
}

func TestRenderServicePodsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	svc := &model.ServiceData{Name: "svc-x", Namespace: "default"}

	// No cluster data at all.
	en.selectedService = svc
	got := en.renderServicePods(svc)
	want := "Backing Pods\n\n  No pod information available"
	if got != want {
		t.Errorf("en no-cluster-data render:\n got %q\nwant %q", got, want)
	}

	// Pods exist but the service has no selector.
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = en.renderServicePods(svc)
	want = "Backing Pods\n\n  No selector configured (ExternalName, manual Endpoints, or headless service)"
	if got != want {
		t.Errorf("en no-selector render:\n got %q\nwant %q", got, want)
	}

	// Selector matches nothing (pods live in another namespace).
	selected := &model.ServiceData{Name: "svc-x", Namespace: "default", Selector: map[string]string{"app": "web"}}
	got = en.renderServicePods(selected)
	want = "Backing Pods (0)\n\n  No pods match this service selector"
	if got != want {
		t.Errorf("en no-match render:\n got %q\nwant %q", got, want)
	}

	// Three matching pods: full byte-exact table.
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web", "extra": "x"}},
		{Name: "pod-b", Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web"}},
		{Name: "pod-c", Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web"}},
	}}
	got = en.renderServicePods(selected)
	want = "Backing Pods (3)\n\n" +
		"  " + "POD NAME" + strings.Repeat(" ", 30) + "  " +
		"PHASE" + strings.Repeat(" ", 6) + "  " +
		"CPU" + strings.Repeat(" ", 8) + "  " +
		"MEMORY" + strings.Repeat(" ", 5) + "  " +
		"RX ↓" + strings.Repeat(" ", 6) + "  " +
		"TX ↑" + strings.Repeat(" ", 6) + "  " +
		"RESTARTS"
	for _, name := range []string{"pod-a", "pod-b", "pod-c"} {
		want += "\n" + "  " + name + strings.Repeat(" ", 33) + "  " +
			"Running" + strings.Repeat(" ", 4) + "  " +
			"-" + strings.Repeat(" ", 10) + "  " +
			"-" + strings.Repeat(" ", 10) + "  " +
			"-" + strings.Repeat(" ", 9) + "  " +
			"-" + strings.Repeat(" ", 9) + "  " +
			"0" + strings.Repeat(" ", 7)
	}
	if got != want {
		t.Errorf("en pods table:\n got %q\nwant %q", got, want)
	}

	// Twelve matching pods: cap at ten rows plus the "... and N more" line.
	var pods []*model.PodData
	for i := 0; i < 12; i++ {
		pods = append(pods, &model.PodData{
			Name:      "pod-" + string(rune('0'+i/10)) + string(rune('0'+i%10)),
			Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web"},
		})
	}
	en.clusterData = &model.ClusterData{Pods: pods}
	got = en.renderServicePods(selected)
	if !strings.Contains(got, "Backing Pods (12)") {
		t.Errorf("en many-pods render %q does not contain the count header", got)
	}
	if !strings.Contains(got, "pod-09") || strings.Contains(got, "pod-10") {
		t.Errorf("en many-pods render must cap at ten rows: %q", got)
	}
	if !strings.HasSuffix(got, "  ... and 2 more pods") {
		t.Errorf("en many-pods render must end with the more-pods line: %q", got)
	}
}

func TestRenderServicePodsZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	svc := &model.ServiceData{Name: "svc-x", Namespace: "default"}
	selected := &model.ServiceData{Name: "svc-x", Namespace: "default", Selector: map[string]string{"app": "web"}}

	got := zh.renderServicePods(svc)
	for _, noun := range []string{"后端 Pod", "暂无 Pod 信息"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-cluster-data render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Backing Pods", "No pod information", "{{", "}}", "<no value", "%!", "service."} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-cluster-data render %q contains %q", got, bad)
		}
	}

	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = zh.renderServicePods(svc)
	for _, noun := range []string{"后端 Pod", "未配置选择器", "无头服务"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-selector render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"ExternalName, manual Endpoints", "headless service", "No selector configured", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-selector render %q contains %q", got, bad)
		}
	}

	got = zh.renderServicePods(selected)
	for _, noun := range []string{"后端 Pod（0）", "没有 Pod 匹配此服务的选择器"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-match render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Backing Pods (0)", "No pods match", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-match render %q contains %q", got, bad)
		}
	}

	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web"}},
		{Name: "pod-b", Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web"}},
		{Name: "pod-c", Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web"}},
	}}
	got = zh.renderServicePods(selected)
	for _, noun := range []string{"后端 Pod（3）", "POD 名称", "阶段", "内存", "接收 ↓", "发送 ↑", "重启次数"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh pods table %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Backing Pods", "PHASE", "MEMORY", "RESTARTS", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh pods table %q contains %q", got, bad)
		}
	}

	var pods []*model.PodData
	for i := 0; i < 12; i++ {
		name := "pod-" + string(rune('0'+i/10)) + string(rune('0'+i%10))
		pods = append(pods, &model.PodData{
			Name: name, Namespace: "default", Phase: "Running", Labels: map[string]string{"app": "web"},
		})
	}
	zh.clusterData = &model.ClusterData{Pods: pods}
	got = zh.renderServicePods(selected)
	if !strings.Contains(got, "后端 Pod（12）") {
		t.Errorf("zh many-pods render %q does not contain the count header", got)
	}
	if !strings.Contains(got, "pod-09") || strings.Contains(got, "pod-10") {
		t.Errorf("zh many-pods render must cap at ten rows: %q", got)
	}
	if !strings.HasSuffix(got, "  ... 还有 2 个 Pod") {
		t.Errorf("zh many-pods render must end with the more-pods line: %q", got)
	}
}

// TestRenderServiceDetailEnGolden locks the outer chrome: the nil-selection
// placeholder and the scroll indicator (short variant, no PgUp/PgDn hint).
func TestRenderServiceDetailEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	if got := en.renderServiceDetail(); got != "No service selected" {
		t.Errorf("en nil-selection = %q, want %q", got, "No service selected")
	}

	svc := &model.ServiceData{
		Name: "svc-x", Namespace: "default", Type: "NodePort",
		CreationTimestamp: time.Now().Add(-5 * time.Minute),
	}
	en.selectedService = svc
	en.clusterData = nil

	en.height = 1000
	tall := en.renderServiceDetail()
	if strings.Contains(tall, "Lines 1-") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", tall)
	}
	total := len(strings.Split(tall, "\n"))

	en.height = 12 // maxVisible = 2
	short := en.renderServiceDetail()
	wantSuffix := "[Lines 1-2 of " + strconv.Itoa(total) + "] (↑/↓ to scroll)"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("short render must end with %q:\n%s", wantSuffix, short)
	}
}

func TestRenderServiceDetailZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	if got := zh.renderServiceDetail(); got != "未选择服务" {
		t.Errorf("zh nil-selection = %q, want 未选择服务", got)
	}

	svc := &model.ServiceData{
		Name: "svc-x", Namespace: "default", Type: "NodePort",
		CreationTimestamp: time.Now().Add(-5 * time.Minute),
	}
	zh.selectedService = svc
	zh.clusterData = nil

	zh.height = 1000
	tall := zh.renderServiceDetail()
	total := len(strings.Split(tall, "\n"))

	zh.height = 12
	short := zh.renderServiceDetail()
	wantSuffix := "[第 1-2 行，共 " + strconv.Itoa(total) + " 行]（↑/↓ 滚动）"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("zh short render must end with %q:\n%s", wantSuffix, short)
	}
	if strings.Contains(short, "↑/↓ to scroll") {
		t.Errorf("zh short render still contains the English scroll hint:\n%s", short)
	}
}

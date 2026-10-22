package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderDaemonSetDetail and its section renderers bilingual lockdown: the
// English output must stay byte-identical to the previously hardcoded strings
// (including leading indentation), and the Chinese output must switch.
// Isomorphic strings render through the shared detail.workload.* keys (see
// workload_detail_shared_keys_test.go); family-specific wording uses
// detail.daemonset.*. Reused keys keep their family values: detail.status
// (the section header, same reuse the cronjob status panel makes),
// detail.namespace / detail.age / detail.scroll_indicator_short and the
// columns.* table headers. The pod Phase values (Running/...) are k8s values
// and stay untranslated. forcedAsciiProfile keeps the goldens plain text.

func daemonSetFixtures(created time.Time) (minimal, full, degraded *model.DaemonSetData) {
	minimal = &model.DaemonSetData{Name: "ds-x", Namespace: "default", CreationTimestamp: created}
	full = &model.DaemonSetData{
		Name: "ds-x", Namespace: "default", CreationTimestamp: created,
		DesiredNumberScheduled: 4, CurrentNumberScheduled: 4, NumberReady: 4, NumberAvailable: 4,
		Labels:   map[string]string{"env": "prod", "team": "api"},
		Selector: map[string]string{"app": "ds-x"},
	}
	degraded = &model.DaemonSetData{
		Name: "ds-x", Namespace: "default",
		DesiredNumberScheduled: 4, CurrentNumberScheduled: 3, NumberReady: 2, NumberAvailable: 2,
	}
	return minimal, full, degraded
}

func daemonSetPod(name, node string, labels map[string]string) *model.PodData {
	return &model.PodData{
		Name: name, Namespace: "default", Node: node, Phase: "Running",
		Labels: labels,
	}
}

func TestRenderDaemonSetBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	// Golden pins "5m", which holds while the fixture-to-render delay stays
	// under 60s — the -5m offset sits exactly at the lower edge of the
	// [5m,6m) window, so any real execution delay only moves deeper into it.
	minimal, full, _ := daemonSetFixtures(time.Now().Add(-5 * time.Minute))

	want := " 👾 DaemonSet: ds-x \n\n" +
		"  Namespace: default\n" +
		"  Age: 5m"
	for name, fixture := range map[string]*model.DaemonSetData{"minimal": minimal, "full": full} {
		if got := en.renderDaemonSetBasicInfo(fixture); got != want {
			t.Errorf("en %s basic info:\n got %q\nwant %q", name, got, want)
		}
	}
}

func TestRenderDaemonSetBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	minimal, _, _ := daemonSetFixtures(time.Now().Add(-5 * time.Minute))

	got := zh.renderDaemonSetBasicInfo(minimal)
	// The DaemonSet kind name translates per the bundle-wide rule (same term
	// as workloads.daemonsets.title); the colon and the labels switch.
	for _, noun := range []string{"👾 守护进程集：ds-x", "命名空间", "年龄"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh basic info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"DaemonSet", "Namespace", "Age:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

func TestRenderDaemonSetStatusEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, full, degraded := daemonSetFixtures(time.Now())

	want := "Status\n\n" +
		"  Desired Number Scheduled: 4\n" +
		"  Current Number Scheduled: 4 / 4\n" +
		"  Number Ready: 4 / 4\n" +
		"  Number Available: 4"
	if got := en.renderDaemonSetStatus(full); got != want {
		t.Errorf("en healthy status:\n got %q\nwant %q", got, want)
	}

	want = "Status\n\n" +
		"  Desired Number Scheduled: 4\n" +
		"  Current Number Scheduled: 3 / 4\n" +
		"  Number Ready: 2 / 4\n" +
		"  Number Available: 2"
	if got := en.renderDaemonSetStatus(degraded); got != want {
		t.Errorf("en degraded status:\n got %q\nwant %q", got, want)
	}
}

func TestRenderDaemonSetStatusZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full, _ := daemonSetFixtures(time.Now())

	got := zh.renderDaemonSetStatus(full)
	for _, noun := range []string{"状态", "期望调度数", "当前调度数", "就绪数", "可用数"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh status %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Status", "Desired Number", "Current Number",
		"Number Ready", "Number Available", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh status %q contains %q", got, bad)
		}
	}
}

func TestRenderDaemonSetSelectorEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	minimal, full, _ := daemonSetFixtures(time.Now())
	empty := en.renderDaemonSetSelector(minimal)
	want := "Selector\n\n  No selector configured\n\nLabels\n\n  No labels"
	if empty != want {
		t.Errorf("en zero-selector render:\n got %q\nwant %q", empty, want)
	}

	got := en.renderDaemonSetSelector(full)
	want = "Selector\n\n" +
		"  app: ds-x\n\n" +
		"Labels\n\n" +
		"  env: prod\n" +
		"  team: api"
	if got != want {
		t.Errorf("en selector render:\n got %q\nwant %q", got, want)
	}
}

func TestRenderDaemonSetSelectorZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	minimal, _, _ := daemonSetFixtures(time.Now())
	got := zh.renderDaemonSetSelector(minimal)
	for _, noun := range []string{"选择器", "未配置选择器", "标签", "无标签"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh zero-selector render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Selector", "No selector", "Labels", "No labels",
		"{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh zero-selector render %q contains %q", got, bad)
		}
	}
}

func TestRenderDaemonSetPodsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, full, _ := daemonSetFixtures(time.Now())

	// No cluster data at all.
	got := en.renderDaemonSetPods(full)
	want := "Managed Pods\n\n  No pod information available"
	if got != want {
		t.Errorf("en no-cluster-data render:\n got %q\nwant %q", got, want)
	}

	// Pods exist but the daemonset has no selector.
	minimal, _, _ := daemonSetFixtures(time.Now())
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = en.renderDaemonSetPods(minimal)
	want = "Managed Pods\n\n  No selector configured (matchLabels is empty)"
	if got != want {
		t.Errorf("en no-selector render:\n got %q\nwant %q", got, want)
	}

	// Selector matches nothing (labels differ).
	got = en.renderDaemonSetPods(full)
	want = "Managed Pods (0)\n\n  No pods match this daemonset selector"
	if got != want {
		t.Errorf("en no-match render:\n got %q\nwant %q", got, want)
	}

	// Two matching pods on their nodes: full byte-exact table (a DaemonSet
	// shows every pod, there is no "... and N more" tail and no restarts
	// column).
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		daemonSetPod("ds-x-abcde", "node-1", map[string]string{"app": "ds-x"}),
		daemonSetPod("ds-x-fghij", "node-2", map[string]string{"app": "ds-x"}),
	}}
	got = en.renderDaemonSetPods(full)
	want = "Managed Pods (2)\n\n" +
		"  " + "POD NAME" + strings.Repeat(" ", 27) + "  " +
		"NODE" + strings.Repeat(" ", 18) + "  " +
		"PHASE" + strings.Repeat(" ", 6) + "  " +
		"CPU" + strings.Repeat(" ", 7) + "  " +
		"MEMORY" + strings.Repeat(" ", 4) + "  " +
		"RX ↓" + strings.Repeat(" ", 5) + "  " +
		"TX ↑" + strings.Repeat(" ", 5) + "\n" +
		"  " + "ds-x-abcde" + strings.Repeat(" ", 25) + "  " +
		"node-1" + strings.Repeat(" ", 16) + "  " +
		"Running" + strings.Repeat(" ", 4) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"-" + strings.Repeat(" ", 8) + "  " +
		"-" + strings.Repeat(" ", 8) + "\n" +
		"  " + "ds-x-fghij" + strings.Repeat(" ", 25) + "  " +
		"node-2" + strings.Repeat(" ", 16) + "  " +
		"Running" + strings.Repeat(" ", 4) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"-" + strings.Repeat(" ", 8) + "  " +
		"-" + strings.Repeat(" ", 8)
	if got != want {
		t.Errorf("en pods table:\n got %q\nwant %q", got, want)
	}
}

func TestRenderDaemonSetPodsZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full, _ := daemonSetFixtures(time.Now())

	got := zh.renderDaemonSetPods(full)
	for _, noun := range []string{"托管 Pod", "暂无 Pod 信息"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-cluster-data render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Managed Pods", "No pod information", "{{", "}}", "<no value", "%!", "detail."} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-cluster-data render %q contains %q", got, bad)
		}
	}

	// Pods exist but the daemonset has no selector.
	minimal, _, _ := daemonSetFixtures(time.Now())
	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = zh.renderDaemonSetPods(minimal)
	for _, noun := range []string{"托管 Pod", "未配置选择器（matchLabels 为空）"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-selector render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Managed Pods", "No selector", "matchLabels is empty", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-selector render %q contains %q", got, bad)
		}
	}

	// Selector matches nothing.
	got = zh.renderDaemonSetPods(full)
	for _, noun := range []string{"托管 Pod（0）", "没有 Pod 匹配此守护进程集的选择器"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-match render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Managed Pods (", "No pods match", "DaemonSet", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-match render %q contains %q", got, bad)
		}
	}

	// Two matching pods: the table headers switch while the pod Phase value
	// stays English on purpose.
	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		daemonSetPod("ds-x-abcde", "node-1", map[string]string{"app": "ds-x"}),
		daemonSetPod("ds-x-fghij", "node-2", map[string]string{"app": "ds-x"}),
	}}
	got = zh.renderDaemonSetPods(full)
	for _, noun := range []string{
		"托管 Pod（2）", "POD 名称", "节点", "阶段", "CPU", "内存", "接收 ↓", "发送 ↑", "Running",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh pods table %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"POD NAME", "NODE", "PHASE", "MEMORY", "RX", "TX",
		"Managed Pods (", "{{", "}}", "<no value", "%!",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh pods table %q contains %q", got, bad)
		}
	}
}

// TestRenderDaemonSetDetailEnGolden locks the outer chrome: the nil-selection
// placeholder and the scroll indicator (short variant, no PgUp/PgDn hint).
func TestRenderDaemonSetDetailEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	if got := en.renderDaemonSetDetail(); got != "No daemonset selected" {
		t.Errorf("en nil-selection = %q, want %q", got, "No daemonset selected")
	}

	_, full, _ := daemonSetFixtures(time.Now().Add(-5 * time.Minute))
	en.selectedDaemonSet = full
	en.clusterData = nil

	en.height = 1000
	tall := en.renderDaemonSetDetail()
	if strings.Contains(tall, "Lines 1-") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", tall)
	}
	total := len(strings.Split(tall, "\n"))

	en.height = 12 // maxVisible = 2
	short := en.renderDaemonSetDetail()
	wantSuffix := "[Lines 1-2 of " + strconv.Itoa(total) + "] (↑/↓ to scroll)"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("short render must end with %q:\n%s", wantSuffix, short)
	}
}

func TestRenderDaemonSetDetailZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	if got := zh.renderDaemonSetDetail(); got != "未选择守护进程集" {
		t.Errorf("zh nil-selection = %q, want 未选择守护进程集", got)
	}

	_, full, _ := daemonSetFixtures(time.Now().Add(-5 * time.Minute))
	zh.selectedDaemonSet = full
	zh.clusterData = nil

	zh.height = 1000
	tall := zh.renderDaemonSetDetail()
	total := len(strings.Split(tall, "\n"))

	zh.height = 12
	short := zh.renderDaemonSetDetail()
	wantSuffix := "[第 1-2 行，共 " + strconv.Itoa(total) + " 行]（↑/↓ 滚动）"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("zh short render must end with %q:\n%s", wantSuffix, short)
	}
	if strings.Contains(short, "↑/↓ to scroll") || strings.Contains(short, "No daemonset selected") || strings.Contains(short, "DaemonSet") {
		t.Errorf("zh short render still contains English chrome:\n%s", short)
	}
}

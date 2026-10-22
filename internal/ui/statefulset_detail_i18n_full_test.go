package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderStatefulSetDetail and its section renderers bilingual lockdown: the
// English output must stay byte-identical to the previously hardcoded strings
// (including leading indentation), and the Chinese output must switch.
// Isomorphic strings render through the shared detail.workload.* keys (see
// workload_detail_shared_keys_test.go); family-specific wording uses
// detail.statefulset.*. Reused keys keep their family values: detail.namespace
// / detail.age / detail.scroll_indicator_short, the columns.* table headers.
// The pod Phase values (Running/...) are k8s values and stay untranslated.
// forcedAsciiProfile keeps the goldens plain text.

func statefulSetFixtures(created time.Time) (minimal, full, degraded *model.StatefulSetData) {
	minimal = &model.StatefulSetData{Name: "sts-x", Namespace: "default", CreationTimestamp: created}
	full = &model.StatefulSetData{
		Name: "sts-x", Namespace: "default", CreationTimestamp: created,
		Replicas: 3, ReadyReplicas: 3, CurrentReplicas: 3, UpdatedReplicas: 3,
		Labels:   map[string]string{"env": "prod", "team": "api"},
		Selector: map[string]string{"app": "sts-x"},
	}
	degraded = &model.StatefulSetData{
		Name: "sts-x", Namespace: "default",
		Replicas: 3, ReadyReplicas: 2, CurrentReplicas: 2, UpdatedReplicas: 1,
	}
	return minimal, full, degraded
}

func statefulSetPod(name string, labels map[string]string, restarts int32) *model.PodData {
	return &model.PodData{
		Name: name, Namespace: "default", Phase: "Running",
		Labels: labels, RestartCount: restarts,
	}
}

func TestRenderStatefulSetBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	// Golden pins "5m", which holds while the fixture-to-render delay stays
	// under 60s — the -5m offset sits exactly at the lower edge of the
	// [5m,6m) window, so any real execution delay only moves deeper into it.
	minimal, full, _ := statefulSetFixtures(time.Now().Add(-5 * time.Minute))

	want := " 📊 StatefulSet: sts-x \n\n" +
		"  Namespace: default\n" +
		"  Age: 5m"
	for name, fixture := range map[string]*model.StatefulSetData{"minimal": minimal, "full": full} {
		if got := en.renderStatefulSetBasicInfo(fixture); got != want {
			t.Errorf("en %s basic info:\n got %q\nwant %q", name, got, want)
		}
	}
}

func TestRenderStatefulSetBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	minimal, _, _ := statefulSetFixtures(time.Now().Add(-5 * time.Minute))

	got := zh.renderStatefulSetBasicInfo(minimal)
	// The StatefulSet kind name translates per the bundle-wide rule (same
	// term as workloads.statefulsets.title); the colon and the labels switch.
	for _, noun := range []string{"📊 有状态副本集：sts-x", "命名空间", "年龄"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh basic info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"StatefulSet", "Namespace", "Age:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

func TestRenderStatefulSetReplicaStatusEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, full, degraded := statefulSetFixtures(time.Now())

	want := "Replica Status\n\n" +
		"  Desired Replicas: 3\n" +
		"  Ready Replicas: 3 / 3\n" +
		"  Current Replicas: 3\n" +
		"  Updated Replicas: 3"
	if got := en.renderStatefulSetReplicaStatus(full); got != want {
		t.Errorf("en healthy replica status:\n got %q\nwant %q", got, want)
	}

	want = "Replica Status\n\n" +
		"  Desired Replicas: 3\n" +
		"  Ready Replicas: 2 / 3\n" +
		"  Current Replicas: 2\n" +
		"  Updated Replicas: 1"
	if got := en.renderStatefulSetReplicaStatus(degraded); got != want {
		t.Errorf("en degraded replica status:\n got %q\nwant %q", got, want)
	}
}

func TestRenderStatefulSetReplicaStatusZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full, _ := statefulSetFixtures(time.Now())

	got := zh.renderStatefulSetReplicaStatus(full)
	for _, noun := range []string{"副本状态", "期望副本数", "就绪副本数", "当前副本数", "已更新副本数"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh replica status %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Replica Status", "Desired Replicas", "Ready Replicas",
		"Current Replicas", "Updated Replicas", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh replica status %q contains %q", got, bad)
		}
	}
}

func TestRenderStatefulSetSelectorEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	minimal, full, _ := statefulSetFixtures(time.Now())
	empty := en.renderStatefulSetSelector(minimal)
	want := "Selector\n\n  No selector configured\n\nLabels\n\n  No labels"
	if empty != want {
		t.Errorf("en zero-selector render:\n got %q\nwant %q", empty, want)
	}

	got := en.renderStatefulSetSelector(full)
	want = "Selector\n\n" +
		"  app: sts-x\n\n" +
		"Labels\n\n" +
		"  env: prod\n" +
		"  team: api"
	if got != want {
		t.Errorf("en selector render:\n got %q\nwant %q", got, want)
	}
}

func TestRenderStatefulSetSelectorZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	minimal, _, _ := statefulSetFixtures(time.Now())
	got := zh.renderStatefulSetSelector(minimal)
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

func TestRenderStatefulSetPodsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, full, _ := statefulSetFixtures(time.Now())

	// No cluster data at all.
	got := en.renderStatefulSetPods(full)
	want := "Managed Pods\n\n  No pod information available"
	if got != want {
		t.Errorf("en no-cluster-data render:\n got %q\nwant %q", got, want)
	}

	// Pods exist but the statefulset has no selector.
	minimal, _, _ := statefulSetFixtures(time.Now())
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = en.renderStatefulSetPods(minimal)
	want = "Managed Pods\n\n  No selector configured (matchLabels is empty)"
	if got != want {
		t.Errorf("en no-selector render:\n got %q\nwant %q", got, want)
	}

	// Selector matches nothing (labels differ).
	got = en.renderStatefulSetPods(full)
	want = "Managed Pods (0)\n\n  No pods match this statefulset selector"
	if got != want {
		t.Errorf("en no-match render:\n got %q\nwant %q", got, want)
	}

	// Two matching pods: full byte-exact table.
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		statefulSetPod("sts-x-aaa", map[string]string{"app": "sts-x"}, 0),
		statefulSetPod("sts-x-bbb", map[string]string{"app": "sts-x"}, 2),
	}}
	got = en.renderStatefulSetPods(full)
	want = "Managed Pods (2)\n\n" +
		"  " + "POD NAME" + strings.Repeat(" ", 30) + "  " +
		"PHASE" + strings.Repeat(" ", 6) + "  " +
		"CPU" + strings.Repeat(" ", 8) + "  " +
		"MEMORY" + strings.Repeat(" ", 5) + "  " +
		"RX ↓" + strings.Repeat(" ", 6) + "  " +
		"TX ↑" + strings.Repeat(" ", 6) + "  " +
		"RESTARTS" + "\n" +
		"  " + "sts-x-aaa" + strings.Repeat(" ", 29) + "  " +
		"Running" + strings.Repeat(" ", 4) + "  " +
		"-" + strings.Repeat(" ", 10) + "  " +
		"-" + strings.Repeat(" ", 10) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"0" + strings.Repeat(" ", 7) + "\n" +
		"  " + "sts-x-bbb" + strings.Repeat(" ", 29) + "  " +
		"Running" + strings.Repeat(" ", 4) + "  " +
		"-" + strings.Repeat(" ", 10) + "  " +
		"-" + strings.Repeat(" ", 10) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"2" + strings.Repeat(" ", 7)
	if got != want {
		t.Errorf("en pods table:\n got %q\nwant %q", got, want)
	}

	// Twenty-two matching pods: cap at twenty rows plus the "... and N more" line.
	var pods []*model.PodData
	for i := 0; i < 22; i++ {
		pods = append(pods, statefulSetPod("sts-x-j"+strconv.Itoa(i), map[string]string{"app": "sts-x"}, 0))
	}
	en.clusterData = &model.ClusterData{Pods: pods}
	got = en.renderStatefulSetPods(full)
	if !strings.Contains(got, "Managed Pods (22)") {
		t.Errorf("en many-pods render %q does not contain the count header", got)
	}
	if !strings.Contains(got, "sts-x-j19") || strings.Contains(got, "sts-x-j20") {
		t.Errorf("en many-pods render must cap at twenty rows: %q", got)
	}
	if !strings.HasSuffix(got, "  ... and 2 more pods") {
		t.Errorf("en many-pods render must end with the more-pods line: %q", got)
	}
}

func TestRenderStatefulSetPodsZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full, _ := statefulSetFixtures(time.Now())

	got := zh.renderStatefulSetPods(full)
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

	// Pods exist but the statefulset has no selector.
	minimal, _, _ := statefulSetFixtures(time.Now())
	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = zh.renderStatefulSetPods(minimal)
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
	got = zh.renderStatefulSetPods(full)
	for _, noun := range []string{"托管 Pod（0）", "没有 Pod 匹配此有状态副本集的选择器"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-match render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Managed Pods (", "No pods match", "StatefulSet", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-match render %q contains %q", got, bad)
		}
	}

	// Two matching pods: the table headers switch while the pod Phase value
	// stays English on purpose.
	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		statefulSetPod("sts-x-aaa", map[string]string{"app": "sts-x"}, 0),
		statefulSetPod("sts-x-bbb", map[string]string{"app": "sts-x"}, 2),
	}}
	got = zh.renderStatefulSetPods(full)
	for _, noun := range []string{
		"托管 Pod（2）", "POD 名称", "阶段", "CPU", "内存", "接收 ↓", "发送 ↑", "重启次数", "Running",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh pods table %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"POD NAME", "PHASE", "MEMORY", "RESTARTS", "RX", "TX",
		"Managed Pods (", "{{", "}}", "<no value", "%!",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh pods table %q contains %q", got, bad)
		}
	}

	// Twenty-two matching pods: capped tail, Chinese wording.
	var pods []*model.PodData
	for i := 0; i < 22; i++ {
		pods = append(pods, statefulSetPod("sts-x-j"+strconv.Itoa(i), map[string]string{"app": "sts-x"}, 0))
	}
	zh.clusterData = &model.ClusterData{Pods: pods}
	got = zh.renderStatefulSetPods(full)
	if !strings.HasSuffix(got, "  ... 还有 2 个 Pod") {
		t.Errorf("zh many-pods render must end with the more-pods line: %q", got)
	}
	if strings.Contains(got, "more pods") {
		t.Errorf("zh many-pods render %q contains the English more-pods text", got)
	}
}

// TestRenderStatefulSetDetailEnGolden locks the outer chrome: the nil-selection
// placeholder and the scroll indicator (short variant, no PgUp/PgDn hint).
func TestRenderStatefulSetDetailEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	if got := en.renderStatefulSetDetail(); got != "No statefulset selected" {
		t.Errorf("en nil-selection = %q, want %q", got, "No statefulset selected")
	}

	_, full, _ := statefulSetFixtures(time.Now().Add(-5 * time.Minute))
	en.selectedStatefulSet = full
	en.clusterData = nil

	en.height = 1000
	tall := en.renderStatefulSetDetail()
	if strings.Contains(tall, "Lines 1-") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", tall)
	}
	total := len(strings.Split(tall, "\n"))

	en.height = 12 // maxVisible = 2
	short := en.renderStatefulSetDetail()
	wantSuffix := "[Lines 1-2 of " + strconv.Itoa(total) + "] (↑/↓ to scroll)"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("short render must end with %q:\n%s", wantSuffix, short)
	}
}

func TestRenderStatefulSetDetailZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	if got := zh.renderStatefulSetDetail(); got != "未选择有状态副本集" {
		t.Errorf("zh nil-selection = %q, want 未选择有状态副本集", got)
	}

	_, full, _ := statefulSetFixtures(time.Now().Add(-5 * time.Minute))
	zh.selectedStatefulSet = full
	zh.clusterData = nil

	zh.height = 1000
	tall := zh.renderStatefulSetDetail()
	total := len(strings.Split(tall, "\n"))

	zh.height = 12
	short := zh.renderStatefulSetDetail()
	wantSuffix := "[第 1-2 行，共 " + strconv.Itoa(total) + " 行]（↑/↓ 滚动）"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("zh short render must end with %q:\n%s", wantSuffix, short)
	}
	if strings.Contains(short, "↑/↓ to scroll") || strings.Contains(short, "No statefulset selected") || strings.Contains(short, "StatefulSet") {
		t.Errorf("zh short render still contains English chrome:\n%s", short)
	}
}

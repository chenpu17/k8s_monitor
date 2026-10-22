package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderDeploymentDetail and its section renderers bilingual lockdown: the
// English output must stay byte-identical to the previously hardcoded strings
// (including leading indentation), and the Chinese output must switch.
// Isomorphic strings render through the shared detail.workload.* keys (see
// workload_detail_shared_keys_test.go); family-specific wording uses
// detail.deployment.*. Reused keys keep their family values: detail.namespace
// / detail.age / detail.scroll_indicator_short, the columns.* table headers.
// The pod Phase values (Running/...) and the strategy values
// (RollingUpdate/Recreate) are k8s values and stay untranslated.
// forcedAsciiProfile keeps the goldens plain text.

func deploymentFixtures(created time.Time) (minimal, full, degraded *model.DeploymentData) {
	minimal = &model.DeploymentData{Name: "deploy-x", Namespace: "default", CreationTimestamp: created}
	full = &model.DeploymentData{
		Name: "deploy-x", Namespace: "default", CreationTimestamp: created,
		Replicas: 3, ReadyReplicas: 3, AvailableReplicas: 3, UpdatedReplicas: 3,
		Strategy: "RollingUpdate",
		Labels:   map[string]string{"env": "prod", "team": "api"},
		Selector: map[string]string{"app": "deploy-x"},
	}
	degraded = &model.DeploymentData{
		Name: "deploy-x", Namespace: "default",
		Replicas: 3, ReadyReplicas: 2, AvailableReplicas: 1, UpdatedReplicas: 1,
		Strategy: "Recreate",
	}
	return minimal, full, degraded
}

func deploymentPod(name string, labels map[string]string, restarts int32) *model.PodData {
	return &model.PodData{
		Name: name, Namespace: "default", Phase: "Running",
		Labels: labels, RestartCount: restarts,
	}
}

func TestRenderDeploymentBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	// Golden pins "5m", which holds while the fixture-to-render delay stays
	// under 60s — the -5m offset sits exactly at the lower edge of the
	// [5m,6m) window, so any real execution delay only moves deeper into it.
	minimal, full, _ := deploymentFixtures(time.Now().Add(-5 * time.Minute))

	want := " 🚀 Deployment: deploy-x \n\n" +
		"  Namespace: default\n" +
		"  Age: 5m"
	for name, fixture := range map[string]*model.DeploymentData{"minimal": minimal, "full": full} {
		if got := en.renderDeploymentBasicInfo(fixture); got != want {
			t.Errorf("en %s basic info:\n got %q\nwant %q", name, got, want)
		}
	}
}

func TestRenderDeploymentBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	minimal, _, _ := deploymentFixtures(time.Now().Add(-5 * time.Minute))

	got := zh.renderDeploymentBasicInfo(minimal)
	// The Deployment kind name translates per the bundle-wide rule (same term
	// as workloads.deployments.title); the colon and the labels switch.
	for _, noun := range []string{"🚀 部署：deploy-x", "命名空间", "年龄"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh basic info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Deployment", "Namespace", "Age:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

func TestRenderDeploymentReplicaStatusEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, full, degraded := deploymentFixtures(time.Now())

	want := "Replica Status\n\n" +
		"  Desired Replicas: 3\n" +
		"  Ready Replicas: 3 / 3\n" +
		"  Updated Replicas: 3\n" +
		"  Available Replicas: 3"
	if got := en.renderDeploymentReplicaStatus(full); got != want {
		t.Errorf("en healthy replica status:\n got %q\nwant %q", got, want)
	}

	want = "Replica Status\n\n" +
		"  Desired Replicas: 3\n" +
		"  Ready Replicas: 2 / 3\n" +
		"  Updated Replicas: 1\n" +
		"  Available Replicas: 1"
	if got := en.renderDeploymentReplicaStatus(degraded); got != want {
		t.Errorf("en degraded replica status:\n got %q\nwant %q", got, want)
	}
}

func TestRenderDeploymentReplicaStatusZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full, _ := deploymentFixtures(time.Now())

	got := zh.renderDeploymentReplicaStatus(full)
	for _, noun := range []string{"副本状态", "期望副本数", "就绪副本数", "已更新副本数", "可用副本数"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh replica status %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Replica Status", "Desired Replicas", "Ready Replicas",
		"Updated Replicas", "Available Replicas", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh replica status %q contains %q", got, bad)
		}
	}
}

func TestRenderDeploymentStrategyEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, full, degraded := deploymentFixtures(time.Now())

	want := "Deployment Strategy\n\n  Type: RollingUpdate"
	if got := en.renderDeploymentStrategy(full); got != want {
		t.Errorf("en RollingUpdate strategy:\n got %q\nwant %q", got, want)
	}

	want = "Deployment Strategy\n\n  Type: Recreate"
	if got := en.renderDeploymentStrategy(degraded); got != want {
		t.Errorf("en Recreate strategy:\n got %q\nwant %q", got, want)
	}

	// An empty strategy falls back to the k8s default value, untranslated.
	empty := &model.DeploymentData{Name: "deploy-x", Namespace: "default"}
	want = "Deployment Strategy\n\n  Type: RollingUpdate"
	if got := en.renderDeploymentStrategy(empty); got != want {
		t.Errorf("en default strategy:\n got %q\nwant %q", got, want)
	}
}

func TestRenderDeploymentStrategyZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full, _ := deploymentFixtures(time.Now())

	got := zh.renderDeploymentStrategy(full)
	// The strategy value is a k8s value and stays untranslated on purpose.
	for _, noun := range []string{"部署策略", "类型", "RollingUpdate"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh strategy %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Deployment Strategy", "Type:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh strategy %q contains %q", got, bad)
		}
	}
}

func TestRenderDeploymentSelectorEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	minimal, full, _ := deploymentFixtures(time.Now())
	empty := en.renderDeploymentSelector(minimal)
	want := "Selector\n\n  No selector configured\n\nLabels\n\n  No labels"
	if empty != want {
		t.Errorf("en zero-selector render:\n got %q\nwant %q", empty, want)
	}

	got := en.renderDeploymentSelector(full)
	want = "Selector\n\n" +
		"  app: deploy-x\n\n" +
		"Labels\n\n" +
		"  env: prod\n" +
		"  team: api"
	if got != want {
		t.Errorf("en selector render:\n got %q\nwant %q", got, want)
	}
}

func TestRenderDeploymentSelectorZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	minimal, _, _ := deploymentFixtures(time.Now())
	got := zh.renderDeploymentSelector(minimal)
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

func TestRenderDeploymentPodsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, full, _ := deploymentFixtures(time.Now())

	// No cluster data at all.
	got := en.renderDeploymentPods(full)
	want := "Managed Pods\n\n  No pod information available"
	if got != want {
		t.Errorf("en no-cluster-data render:\n got %q\nwant %q", got, want)
	}

	// Pods exist but the deployment has no selector.
	minimal, _, _ := deploymentFixtures(time.Now())
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = en.renderDeploymentPods(minimal)
	want = "Managed Pods\n\n  No selector configured (matchLabels is empty)"
	if got != want {
		t.Errorf("en no-selector render:\n got %q\nwant %q", got, want)
	}

	// Selector matches nothing (labels differ).
	got = en.renderDeploymentPods(full)
	want = "Managed Pods (0)\n\n  No pods match this deployment selector"
	if got != want {
		t.Errorf("en no-match render:\n got %q\nwant %q", got, want)
	}

	// Two matching pods: full byte-exact table.
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{
		deploymentPod("deploy-x-aaa", map[string]string{"app": "deploy-x"}, 0),
		deploymentPod("deploy-x-bbb", map[string]string{"app": "deploy-x"}, 2),
	}}
	got = en.renderDeploymentPods(full)
	want = "Managed Pods (2)\n\n" +
		"  " + "POD NAME" + strings.Repeat(" ", 30) + "  " +
		"PHASE" + strings.Repeat(" ", 6) + "  " +
		"CPU" + strings.Repeat(" ", 8) + "  " +
		"MEMORY" + strings.Repeat(" ", 5) + "  " +
		"RX ↓" + strings.Repeat(" ", 6) + "  " +
		"TX ↑" + strings.Repeat(" ", 6) + "  " +
		"RESTARTS" + "\n" +
		"  " + "deploy-x-aaa" + strings.Repeat(" ", 26) + "  " +
		"Running" + strings.Repeat(" ", 4) + "  " +
		"-" + strings.Repeat(" ", 10) + "  " +
		"-" + strings.Repeat(" ", 10) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"-" + strings.Repeat(" ", 9) + "  " +
		"0" + strings.Repeat(" ", 7) + "\n" +
		"  " + "deploy-x-bbb" + strings.Repeat(" ", 26) + "  " +
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
		pods = append(pods, deploymentPod("deploy-x-j"+strconv.Itoa(i), map[string]string{"app": "deploy-x"}, 0))
	}
	en.clusterData = &model.ClusterData{Pods: pods}
	got = en.renderDeploymentPods(full)
	if !strings.Contains(got, "Managed Pods (22)") {
		t.Errorf("en many-pods render %q does not contain the count header", got)
	}
	if !strings.Contains(got, "deploy-x-j19") || strings.Contains(got, "deploy-x-j20") {
		t.Errorf("en many-pods render must cap at twenty rows: %q", got)
	}
	if !strings.HasSuffix(got, "  ... and 2 more pods") {
		t.Errorf("en many-pods render must end with the more-pods line: %q", got)
	}
}

func TestRenderDeploymentPodsZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full, _ := deploymentFixtures(time.Now())

	got := zh.renderDeploymentPods(full)
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

	// Pods exist but the deployment has no selector.
	minimal, _, _ := deploymentFixtures(time.Now())
	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Phase: "Running"},
	}}
	got = zh.renderDeploymentPods(minimal)
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
	got = zh.renderDeploymentPods(full)
	for _, noun := range []string{"托管 Pod（0）", "没有 Pod 匹配此部署的选择器"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-match render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Managed Pods (", "No pods match", "Deployment", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-match render %q contains %q", got, bad)
		}
	}

	// Two matching pods: the table headers switch while the pod Phase value
	// stays English on purpose.
	zh.clusterData = &model.ClusterData{Pods: []*model.PodData{
		deploymentPod("deploy-x-aaa", map[string]string{"app": "deploy-x"}, 0),
		deploymentPod("deploy-x-bbb", map[string]string{"app": "deploy-x"}, 2),
	}}
	got = zh.renderDeploymentPods(full)
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
		pods = append(pods, deploymentPod("deploy-x-j"+strconv.Itoa(i), map[string]string{"app": "deploy-x"}, 0))
	}
	zh.clusterData = &model.ClusterData{Pods: pods}
	got = zh.renderDeploymentPods(full)
	if !strings.HasSuffix(got, "  ... 还有 2 个 Pod") {
		t.Errorf("zh many-pods render must end with the more-pods line: %q", got)
	}
	if strings.Contains(got, "more pods") {
		t.Errorf("zh many-pods render %q contains the English more-pods text", got)
	}
}

// TestRenderDeploymentDetailEnGolden locks the outer chrome: the nil-selection
// placeholder and the scroll indicator (short variant, no PgUp/PgDn hint).
func TestRenderDeploymentDetailEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	if got := en.renderDeploymentDetail(); got != "No deployment selected" {
		t.Errorf("en nil-selection = %q, want %q", got, "No deployment selected")
	}

	_, full, _ := deploymentFixtures(time.Now().Add(-5 * time.Minute))
	en.selectedDeployment = full
	en.clusterData = nil

	en.height = 1000
	tall := en.renderDeploymentDetail()
	if strings.Contains(tall, "Lines 1-") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", tall)
	}
	total := len(strings.Split(tall, "\n"))

	en.height = 12 // maxVisible = 2
	short := en.renderDeploymentDetail()
	wantSuffix := "[Lines 1-2 of " + strconv.Itoa(total) + "] (↑/↓ to scroll)"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("short render must end with %q:\n%s", wantSuffix, short)
	}
}

func TestRenderDeploymentDetailZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	if got := zh.renderDeploymentDetail(); got != "未选择部署" {
		t.Errorf("zh nil-selection = %q, want 未选择部署", got)
	}

	_, full, _ := deploymentFixtures(time.Now().Add(-5 * time.Minute))
	zh.selectedDeployment = full
	zh.clusterData = nil

	zh.height = 1000
	tall := zh.renderDeploymentDetail()
	total := len(strings.Split(tall, "\n"))

	zh.height = 12
	short := zh.renderDeploymentDetail()
	wantSuffix := "[第 1-2 行，共 " + strconv.Itoa(total) + " 行]（↑/↓ 滚动）"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("zh short render must end with %q:\n%s", wantSuffix, short)
	}
	if strings.Contains(short, "↑/↓ to scroll") || strings.Contains(short, "No deployment selected") || strings.Contains(short, "Deployment") {
		t.Errorf("zh short render still contains English chrome:\n%s", short)
	}
}

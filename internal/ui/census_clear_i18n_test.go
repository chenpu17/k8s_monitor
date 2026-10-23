package ui

// census_clear_i18n_test.go — batch 19 (i18n census clear-out) lockdowns.
//
// The batch keyed the last hardcoded user-facing sites: the action menu
// family (menu items, title, help, executeAction output titles/contents),
// the overview CPU/Memory panel headers, the nodes NPU summary prefix, the
// workloads scheduling-efficiency label, the command-output scroll
// indicator, the logs search footer trio and no-containers error, the
// update feedback messages (export/clipboard), the export error strings
// (keeping their ": %w" separators so errors.Is still unwraps), the
// fetchLogs provider error, and the header "by" line.
//
// The en goldens below are byte-for-byte the pre-batch hardcoded strings
// (verified against a detached HEAD dump with cmp before being frozen here).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// censusPod is a pod fixture with one container; censusBarePod has none.
func censusPod() *model.PodData {
	return &model.PodData{
		Name: "web-0", Namespace: "default", Phase: "Running",
		ContainerStates: []model.ContainerState{{Name: "app"}},
	}
}

func censusBarePod() *model.PodData {
	return &model.PodData{Name: "web-0", Namespace: "default"}
}

func censusNode() *model.NodeData {
	return &model.NodeData{Name: "node-1"}
}

// censusDumpProvider implements the anonymous describe/YAML interfaces
// executeAction type-asserts on.
type censusDumpProvider struct {
	describeOut string
	describeErr error
	yamlOut     string
	yamlErr     error
}

func (p *censusDumpProvider) GetClusterData() (*model.ClusterData, error) { return nil, nil }
func (p *censusDumpProvider) ForceRefresh() error                         { return nil }
func (p *censusDumpProvider) DescribePod(ctx context.Context, ns, name string) (string, error) {
	return p.describeOut, p.describeErr
}
func (p *censusDumpProvider) DescribeNode(ctx context.Context, name string) (string, error) {
	return p.describeOut, p.describeErr
}
func (p *censusDumpProvider) GetPodYAML(ctx context.Context, ns, name string) (string, error) {
	return p.yamlOut, p.yamlErr
}
func (p *censusDumpProvider) GetNodeYAML(ctx context.Context, name string) (string, error) {
	return p.yamlOut, p.yamlErr
}

// censusLogsFixture builds a model with a 15-line log body; height 20 shows
// 12 of the 15 lines, exercising the scroll footer.
func censusLogsFixture(t *testing.T, locale string) *Model {
	t.Helper()
	forcedAsciiProfile(t)
	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.height = 20
	m.selectedPod = censusPod()
	m.selectedContainer = "app"
	m.containerLogs = strings.Join([]string{
		"alpha line one", "beta line two", "gamma line three", "alpha again",
		"delta line five", "epsilon line six", "zeta line seven", "eta line eight",
		"theta line nine", "iota line ten", "kappa line eleven", "lambda line twelve",
		"mu line thirteen", "nu line fourteen", "xi line fifteen",
	}, "\n")
	return m
}

// TestCensusClearActionMenuEnGolden pins the English bytes of the action
// menu in three full forms: pod detail with the first item selected (its
// description row shows), pod detail with a kubectl-command description
// selected, and the node-detail action set.
func TestCensusClearActionMenuEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.currentView = ViewPodDetail
	en.detailMode = true
	en.actionMenuMode = true
	en.selectedPod = censusPod()

	en.actionMenuSelectedIndex = 0
	wantPodFirst := "╭──────────────────────────────────────────────╮\n│                                              │\n│   🎯 Quick Actions                           │\n│                                              │\n│    1  📜 View Logs                           │\n│       Show container logs                    │\n│    2  📋 Describe                            │\n│    3  📄 Get YAML                            │\n│    4  📎 Copy Name                           │\n│    5  📎 Copy Namespace/Name                 │\n│    6  ⚡ Show Events                         │\n│                                              │\n│    ↑/↓ Navigate • Enter Select • ESC Cancel  │\n│                                              │\n╰──────────────────────────────────────────────╯"
	if got := en.renderActionMenu(); got != wantPodFirst {
		t.Errorf("en action menu (pod, first selected):\n got %q\nwant %q", got, wantPodFirst)
	}

	en.actionMenuSelectedIndex = 2
	wantPodYAML := "╭──────────────────────────────────────────────╮\n│                                              │\n│   🎯 Quick Actions                           │\n│                                              │\n│    1  📜 View Logs                           │\n│    2  📋 Describe                            │\n│    3  📄 Get YAML                            │\n│       kubectl get -o yaml                    │\n│    4  📎 Copy Name                           │\n│    5  📎 Copy Namespace/Name                 │\n│    6  ⚡ Show Events                         │\n│                                              │\n│    ↑/↓ Navigate • Enter Select • ESC Cancel  │\n│                                              │\n╰──────────────────────────────────────────────╯"
	if got := en.renderActionMenu(); got != wantPodYAML {
		t.Errorf("en action menu (pod, Get YAML selected):\n got %q\nwant %q", got, wantPodYAML)
	}

	en.currentView = ViewNodeDetail
	en.selectedPod = nil
	en.selectedNode = censusNode()
	en.actionMenuSelectedIndex = 1
	wantNodeYAML := "╭──────────────────────────────────────────────╮\n│                                              │\n│   🎯 Quick Actions                           │\n│                                              │\n│    1  📋 Describe                            │\n│    2  📄 Get YAML                            │\n│       kubectl get -o yaml                    │\n│    3  📎 Copy Name                           │\n│    4  ⚡ Show Events                         │\n│                                              │\n│    ↑/↓ Navigate • Enter Select • ESC Cancel  │\n│                                              │\n╰──────────────────────────────────────────────╯"
	if got := en.renderActionMenu(); got != wantNodeYAML {
		t.Errorf("en action menu (node, Get YAML selected):\n got %q\nwant %q", got, wantNodeYAML)
	}

	// The remaining item labels/descriptions appear verbatim somewhere in
	// their menu's render (known-list semantics, not a count). Walk all
	// selection indices — a description row only renders for its own item.
	en.currentView = ViewPodDetail
	en.selectedPod = censusPod()
	en.selectedNode = nil
	podMenu := ""
	for idx := 0; idx < 6; idx++ {
		en.actionMenuSelectedIndex = idx
		podMenu += en.renderActionMenu() + "\n"
	}
	for _, want := range []string{
		"Copy pod name to clipboard", "Copy namespace/name",
		"Copy Namespace/Name", "Related events", "kubectl describe pod",
	} {
		if !strings.Contains(podMenu, want) {
			t.Errorf("en pod action menu missing %q:\n%s", want, podMenu)
		}
	}
	en.currentView = ViewNodeDetail
	en.selectedPod = nil
	en.selectedNode = censusNode()
	nodeMenu := ""
	for idx := 0; idx < 4; idx++ {
		en.actionMenuSelectedIndex = idx
		nodeMenu += en.renderActionMenu() + "\n"
	}
	for _, want := range []string{"kubectl describe node", "Copy node name", "Related events"} {
		if !strings.Contains(nodeMenu, want) {
			t.Errorf("en node action menu missing %q:\n%s", want, nodeMenu)
		}
	}
}

// TestCensusClearExecuteActionEnGolden pins the commandOutputMsg payloads of
// every executeAction output path. The err field of the unsupported paths is
// a sentinel value that Update never renders — asserted for stability only.
func TestCensusClearExecuteActionEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	run := func(m *Model, act ActionType) commandOutputMsg {
		t.Helper()
		cmd := m.executeAction(act)
		if cmd == nil {
			t.Fatalf("executeAction(%d) returned no command", act)
		}
		msg, ok := cmd().(commandOutputMsg)
		if !ok {
			t.Fatalf("executeAction(%d) returned %T", act, cmd())
		}
		return msg
	}

	pod, node := censusPod(), censusNode()

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.selectedPod = pod

	// dataProvider nil: the type assertions fail → unsupported outputs.
	msg := run(m, ActionDescribe)
	if msg.title != "Error" || msg.content != "API client does not support describe functionality" {
		t.Errorf("en describe-unsupported:\n got %q / %q", msg.title, msg.content)
	}
	if msg.err == nil || msg.err.Error() != "unsupported operation" {
		t.Errorf("en describe-unsupported sentinel: got %v", msg.err)
	}
	msg = run(m, ActionGetYAML)
	if msg.title != "Error" || msg.content != "API client does not support YAML export" {
		t.Errorf("en yaml-unsupported:\n got %q / %q", msg.title, msg.content)
	}

	// Success and error paths, pod and node.
	m.dataProvider = &censusDumpProvider{describeOut: "DESC-OUT"}
	if msg := run(m, ActionDescribe); msg.title != "Describe Pod: default/web-0" {
		t.Errorf("en describe-pod title: got %q", msg.title)
	}
	m.dataProvider = &censusDumpProvider{describeErr: errors.New("boom-desc")}
	if msg := run(m, ActionDescribe); msg.title != "Describe Error" || msg.content != "boom-desc" {
		t.Errorf("en describe-pod error:\n got %q / %q", msg.title, msg.content)
	}
	m.selectedPod, m.selectedNode = nil, node
	m.dataProvider = &censusDumpProvider{describeOut: "DESC-NODE"}
	if msg := run(m, ActionDescribe); msg.title != "Describe Node: node-1" {
		t.Errorf("en describe-node title: got %q", msg.title)
	}
	m.selectedPod, m.selectedNode = pod, nil
	m.dataProvider = &censusDumpProvider{yamlOut: "YAML-OUT"}
	if msg := run(m, ActionGetYAML); msg.title != "YAML: default/web-0" {
		t.Errorf("en yaml-pod title: got %q", msg.title)
	}
	m.dataProvider = &censusDumpProvider{yamlErr: errors.New("boom-yaml")}
	if msg := run(m, ActionGetYAML); msg.title != "Get YAML Error" || msg.content != "boom-yaml" {
		t.Errorf("en yaml-pod error:\n got %q / %q", msg.title, msg.content)
	}
	m.selectedPod, m.selectedNode = nil, node
	m.dataProvider = &censusDumpProvider{yamlOut: "YAML-NODE"}
	if msg := run(m, ActionGetYAML); msg.title != "YAML: node-1" {
		t.Errorf("en yaml-node title: got %q", msg.title)
	}
}

// TestCensusClearPanelsEnGolden pins the overview CPU/Memory panels, the
// nodes NPU summary prefix line, the workloads scheduling label, the
// command-output scroll indicator and the header byline in English.
func TestCensusClearPanelsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	sum := &model.ClusterSummary{
		CPUCapacity: 16000, CPUAllocatable: 15400, CPURequested: 4200, CPUUsed: 3100,
		MemoryCapacity: 68719476736, MemoryAllocatable: 64424509440,
		MemoryRequested: 21474836480, MemoryUsed: 12884901888,
	}
	wantCPU := " [⚡ CPU] \n\nCapacity:   16.0\nAllocatable: 15.4\nRequested: 4.2\nActual:   3.1"
	if got := strings.Join(en.cpuDetailsLines(sum), "\n"); got != wantCPU {
		t.Errorf("en CPU panel:\n got %q\nwant %q", got, wantCPU)
	}
	wantMem := " [🧠 Memory] \n\nCapacity:   64.0Gi\nAllocatable: 60.0Gi\nRequested: 20.0Gi\nActual:   12.0Gi"
	if got := strings.Join(en.memoryDetailsLines(sum), "\n"); got != wantMem {
		t.Errorf("en Memory panel:\n got %q\nwant %q", got, wantMem)
	}

	wantNPU := "  🧮 NPU: Allocated: 8/16 • Usage: 50.0% • nodes: 2"
	if got := en.renderNPUClusterSummary(&model.ClusterSummary{
		NPUNodesCount: 2, NPUAllocated: 8, NPUAllocatable: 16, NPUUtilization: 50,
	}); got != wantNPU {
		t.Errorf("en nodes NPU line:\n got %q\nwant %q", got, wantNPU)
	}

	wantSched := "    🧮 NPU Total: 16 • Volcano Running: 8 • Efficiency: 50.0% • Sched: 67%"
	if got := en.renderVolcanoNPUSummary(16, 8, 2, 1); got != wantSched {
		t.Errorf("en workloads NPU summary:\n got %q\nwant %q", got, wantSched)
	}

	// Command-output scroll indicator: two window positions.
	en.height = 20
	en.commandOutputTitle = "Describe Pod: default/web-0"
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, "content line "+string(rune('0'+i/10))+string(rune('0'+i%10)))
	}
	en.commandOutputContent = strings.Join(lines, "\n")
	en.commandOutputScroll = 0
	wantScroll0 := " Describe Pod: default/web-0 \n\n   1│ content line 00\n   2│ content line 01\n   3│ content line 02\n   4│ content line 03\n   5│ content line 04\n   6│ content line 05\n   7│ content line 06\n   8│ content line 07\n   9│ content line 08\n  10│ content line 09\n\nLines 1-10 of 30"
	if got := en.renderCommandOutput(); got != wantScroll0 {
		t.Errorf("en command output scroll=0:\n got %q\nwant %q", got, wantScroll0)
	}
	en.commandOutputScroll = 5
	wantScroll5 := " Describe Pod: default/web-0 \n\n   6│ content line 05\n   7│ content line 06\n   8│ content line 07\n   9│ content line 08\n  10│ content line 09\n  11│ content line 10\n  12│ content line 11\n  13│ content line 12\n  14│ content line 13\n  15│ content line 14\n\nLines 6-15 of 30"
	if got := en.renderCommandOutput(); got != wantScroll5 {
		t.Errorf("en command output scroll=5:\n got %q\nwant %q", got, wantScroll5)
	}

	// Header byline keeps its exact en bytes.
	if got := en.renderHeader(); !strings.Contains(got, "🔍 Kubernetes Monitor  by chenpu  test") {
		t.Errorf("en header byline mismatch:\n%s", got)
	}
}

// TestCensusClearLogsFooterEnGolden pins the three footer states and the
// search-matches indicator of the logs viewer in English bytes.
func TestCensusClearLogsFooterEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	m := censusLogsFixture(t, "en")

	m.logsSearchMode = true
	if got := m.renderLogs(); !strings.HasSuffix(got, "\n[Lines 1-8 of 15] • ⏸ Paused • Loading logs... Type to search • ESC to exit search") {
		t.Errorf("en logs search-hint footer:\n%s", got)
	}
	m.logsSearchText = "alpha"
	if got := m.renderLogs(); !strings.HasSuffix(got, "\n🔍 2/15 matches • ⏸ Paused • Loading logs... Type to search • ESC to exit search") {
		t.Errorf("en logs search-matches footer:\n%s", got)
	}
	m.logsSearchMode = false
	m.logsSearchText = ""
	if got := m.renderLogs(); !strings.HasSuffix(got, "\n[Lines 1-9 of 15] • ⏸ Paused • Loading logs... ↑/↓ scroll • PgUp/PgDn page • Esc back • / to search") {
		t.Errorf("en logs scroll footer:\n%s", got)
	}
	m.containerLogs = "only one line"
	if got := m.renderLogs(); !strings.HasSuffix(got, "\n[1 lines] • ⏸ Paused • Loading logs... Esc back • / to search") {
		t.Errorf("en logs short footer:\n%s", got)
	}
}

// TestCensusClearUpdateFeedbackEnGolden feeds the four feedback messages to
// Update and pins the resulting status strings byte-exactly.
func TestCensusClearUpdateFeedbackEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	m.Update(exportSuccessMsg{count: 3, filePath: "/tmp/export/x.csv"})
	if got := m.exportMessage; got != "✅ Exported 3 items to: /tmp/export/x.csv" {
		t.Errorf("en export success message:\n got %q", got)
	}
	m.Update(exportErrorMsg{err: errors.New("disk full")})
	if got := m.exportMessage; got != "❌ Export failed: disk full" {
		t.Errorf("en export error message:\n got %q", got)
	}
	m.Update(clipboardResultMsg{text: "web-0"})
	if got := m.exportMessage; got != "✅ Copied: web-0" {
		t.Errorf("en clipboard success message:\n got %q", got)
	}
	m.Update(clipboardResultMsg{err: errors.New("no clipboard tool")})
	if got := m.exportMessage; got != "❌ Copy failed: no clipboard tool" {
		t.Errorf("en clipboard error message:\n got %q", got)
	}
}

// TestCensusClearLogsErrorEnGolden drives both no-containers sites through
// the l key handler and pins the error text.
func TestCensusClearLogsErrorEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	pressL := func(m *Model) {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	}

	// Pod detail site.
	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.currentView = ViewPodDetail
	en.detailMode = true
	en.selectedPod = censusBarePod()
	pressL(en)
	if got := en.logsError; got != "No containers available" {
		t.Errorf("en pod-detail logsError:\n got %q", got)
	}

	// Job detail site (job has one pod without containers).
	en = NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.currentView = ViewJobDetail
	en.detailMode = true
	en.selectedJob = &model.JobData{Name: "web", Namespace: "default"}
	en.clusterData = &model.ClusterData{Pods: []*model.PodData{censusBarePod()}}
	pressL(en)
	if got := en.logsError; got != "No containers available" {
		t.Errorf("en job-detail logsError:\n got %q", got)
	}
}

// TestCensusClearExportErrorsEnGolden triggers every export error and pins
// the English bytes. The two filesystem errors also prove %w fidelity via
// errors.Is (the ": %w" separator stays in code after keying).
func TestCensusClearExportErrorsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	// mkdir failure: /sys is a read-only filesystem, so MkdirAll fails while
	// Stat still reports IsNotExist (works for root and non-root alike).
	t.Setenv("HOME", "/sys")
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, err := m.getExportDir()
	if err == nil {
		t.Fatal("getExportDir with HOME=/sys returned no error")
	}
	if !strings.HasPrefix(err.Error(), "cannot create export directory /sys/.config/k8s-monitor/exports: ") {
		t.Errorf("en dir-create error prefix:\n got %q", err.Error())
	}
	if !errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.EROFS) {
		t.Errorf("en dir-create error lost the wrapped fs error: %v", err)
	}

	// Write-test failure: HOME is a regular file, so the write probe fails
	// with ENOTDIR at a deterministic path.
	t.Setenv("HOME", "/dev/null")
	m = NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, err = m.getExportDir()
	if err == nil {
		t.Fatal("getExportDir with HOME=/dev/null returned no error")
	}
	want := "export directory /dev/null/.config/k8s-monitor/exports is not writable: open /dev/null/.config/k8s-monitor/exports/.write_test: not a directory"
	if got := err.Error(); got != want {
		t.Errorf("en dir-not-writable error:\n got %q\nwant %q", got, want)
	}
	if !errors.Is(err, syscall.ENOTDIR) {
		t.Errorf("en dir-not-writable error lost the wrapped ENOTDIR: %v", err)
	}

	// View-unsupported and the four empty-view errors.
	t.Setenv("HOME", t.TempDir())
	m = NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.currentView = ViewOverview
	msg, ok := m.exportData(ExportCSV)().(exportErrorMsg)
	if !ok {
		t.Fatalf("overview export returned %T", m.exportData(ExportCSV)())
	}
	if got := msg.err.Error(); got != "export not supported for this view" {
		t.Errorf("en view-unsupported error:\n got %q", got)
	}

	for _, tc := range []struct {
		view ViewType
		want string
	}{
		{ViewNodes, "no nodes data to export"},
		{ViewPods, "no pods data to export"},
		{ViewEvents, "no events data to export"},
		{ViewNetwork, "no services data to export"},
	} {
		m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
		m.currentView = tc.view
		m.clusterData = &model.ClusterData{}
		em, ok := m.exportData(ExportCSV)().(exportErrorMsg)
		if !ok {
			t.Fatalf("export for view %d returned a non-error message", tc.view)
		}
		if got := em.err.Error(); got != tc.want {
			t.Errorf("en no-data error for view %d:\n got %q\nwant %q", tc.view, got, tc.want)
		}
	}
}

// TestCensusClearFetchLogsProviderEnGolden pins the provider-unsupported
// fetch error (surfaced via m.logsError).
func TestCensusClearFetchLogsProviderEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.selectedPod = censusPod()
	en.selectedContainer = "app"
	cmd := en.fetchLogs()
	if cmd == nil {
		t.Fatal("fetchLogs returned no command")
	}
	lm, ok := cmd().(logsMsg)
	if !ok {
		t.Fatalf("fetchLogs returned %T", cmd())
	}
	if lm.err == nil || lm.err.Error() != "data provider does not support log fetching" {
		t.Errorf("en fetchLogs provider error:\n got %v", lm.err)
	}
}

// TestCensusClearZhSwitch verifies the Chinese side switches at every
// batch-19 site. Same-value-on-purpose keys (the NPU prefix, the CPU panel
// header, the kubectl command descriptions) are asserted by presence.
func TestCensusClearZhSwitch(t *testing.T) {
	forcedAsciiProfile(t)

	pressL := func(m *Model) {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	}
	zh := func() *Model { return NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200) }

	// Action menu: labels switch, kubectl commands stay verbatim. Walk all
	// selection indices — a description row only renders for its own item.
	m := zh()
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.actionMenuMode = true
	m.selectedPod = censusPod()
	menu := ""
	for idx := 0; idx < 6; idx++ {
		m.actionMenuSelectedIndex = idx
		menu += m.renderActionMenu() + "\n"
	}
	for _, want := range []string{
		"🎯 快捷操作", "📜 查看日志", "查看容器日志", "📋 描述",
		"📄 获取 YAML", "📎 复制名称", "复制 Pod 名称到剪贴板",
		"📎 复制命名空间/名称", "复制命名空间/名称", "⚡ 查看事件", "相关事件",
		"  ↑/↓ 导航 • Enter 选择 • Esc 取消",
		"kubectl describe pod", "kubectl get -o yaml",
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("zh pod action menu missing %q:\n%s", want, menu)
		}
	}
	for _, bad := range []string{"Quick Actions", "View Logs", "Copy Name", "Navigate •"} {
		if strings.Contains(menu, bad) {
			t.Errorf("zh pod action menu still contains %q:\n%s", bad, menu)
		}
	}

	// executeAction zh payloads.
	m = zh()
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.selectedPod = censusPod()
	if cmd := m.executeAction(ActionDescribe); cmd == nil {
		t.Fatal("describe returned no command")
	} else if msg, ok := cmd().(commandOutputMsg); ok {
		if msg.title != "错误" || msg.content != "API 客户端不支持 describe 功能" {
			t.Errorf("zh describe-unsupported:\n got %q / %q", msg.title, msg.content)
		}
	}
	m.dataProvider = &censusDumpProvider{describeOut: "DESC"}
	if cmd := m.executeAction(ActionDescribe); cmd != nil {
		if msg, ok := cmd().(commandOutputMsg); ok && msg.title != "描述 Pod：default/web-0" {
			t.Errorf("zh describe-pod title:\n got %q", msg.title)
		}
	}
	m.selectedPod, m.selectedNode = nil, censusNode()
	if cmd := m.executeAction(ActionDescribe); cmd != nil {
		if msg, ok := cmd().(commandOutputMsg); ok && msg.title != "描述节点：node-1" {
			t.Errorf("zh describe-node title:\n got %q", msg.title)
		}
	}
	m.selectedPod, m.selectedNode = censusPod(), nil
	m.dataProvider = &censusDumpProvider{yamlOut: "YAML"}
	if cmd := m.executeAction(ActionGetYAML); cmd != nil {
		if msg, ok := cmd().(commandOutputMsg); ok && msg.title != "YAML：default/web-0" {
			t.Errorf("zh yaml-pod title:\n got %q", msg.title)
		}
	}
	m.selectedPod, m.selectedNode = nil, censusNode()
	if cmd := m.executeAction(ActionGetYAML); cmd != nil {
		if msg, ok := cmd().(commandOutputMsg); ok && msg.title != "YAML：node-1" {
			t.Errorf("zh yaml-node title:\n got %q", msg.title)
		}
	}
	m.selectedPod, m.selectedNode = censusPod(), nil
	m.dataProvider = &censusDumpProvider{yamlErr: errors.New("boom")}
	if cmd := m.executeAction(ActionGetYAML); cmd != nil {
		if msg, ok := cmd().(commandOutputMsg); ok && msg.title != "获取 YAML 出错" {
			t.Errorf("zh yaml error title:\n got %q", msg.title)
		}
	}
	m.dataProvider = &censusDumpProvider{describeErr: errors.New("boom")}
	if cmd := m.executeAction(ActionDescribe); cmd != nil {
		if msg, ok := cmd().(commandOutputMsg); ok && msg.title != "描述出错" {
			t.Errorf("zh describe error title:\n got %q", msg.title)
		}
	}
	m.dataProvider = nil
	m.selectedPod = censusPod()
	if cmd := m.executeAction(ActionGetYAML); cmd != nil {
		if msg, ok := cmd().(commandOutputMsg); ok && msg.content != "API 客户端不支持 YAML 导出" {
			t.Errorf("zh yaml-unsupported content:\n got %q", msg.content)
		}
	}

	// Overview panels: CPU is same-value on purpose, Memory translates.
	m = zh()
	sum := &model.ClusterSummary{
		CPUCapacity: 16000, CPUAllocatable: 15400, CPURequested: 4200, CPUUsed: 3100,
		MemoryCapacity: 68719476736, MemoryAllocatable: 64424509440,
		MemoryRequested: 21474836480, MemoryUsed: 12884901888,
	}
	cpu := strings.Join(m.cpuDetailsLines(sum), "\n")
	if !strings.Contains(cpu, "[⚡ CPU]") {
		t.Errorf("zh CPU panel missing the same-value header:\n%s", cpu)
	}
	mem := strings.Join(m.memoryDetailsLines(sum), "\n")
	if !strings.Contains(mem, "[🧠 内存]") || strings.Contains(mem, "[🧠 Memory]") {
		t.Errorf("zh Memory panel did not switch:\n%s", mem)
	}

	// Nodes NPU prefix: same-value on purpose.
	if got := m.renderNPUClusterSummary(&model.ClusterSummary{
		NPUNodesCount: 2, NPUAllocated: 8, NPUAllocatable: 16, NPUUtilization: 50,
	}); !strings.HasPrefix(got, "  🧮 NPU: ") {
		t.Errorf("zh nodes NPU line lost the same-value prefix:\n%s", got)
	}

	// Workloads scheduling label switches.
	sched := m.renderVolcanoNPUSummary(16, 8, 2, 1)
	if !strings.Contains(sched, "调度: 67%") || strings.Contains(sched, "Sched: ") {
		t.Errorf("zh workloads NPU summary did not switch:\n%s", sched)
	}

	// Command-output scroll indicator.
	m = zh()
	m.height = 20
	m.commandOutputTitle = "t"
	var zhLines []string
	for i := 0; i < 30; i++ {
		zhLines = append(zhLines, fmt.Sprintf("内容 %02d", i))
	}
	m.commandOutputContent = strings.Join(zhLines, "\n")
	if got := m.renderCommandOutput(); !strings.Contains(got, "第 1-10 行，共 30 行") {
		t.Errorf("zh command output scroll indicator:\n%s", got)
	}

	// Logs footers switch; en copy must not leak.
	m = censusLogsFixture(t, "zh")
	m.logsSearchMode = true
	logs := m.renderLogs()
	if !strings.Contains(logs, "输入以搜索 • Esc 退出搜索") || strings.Contains(logs, "Type to search") {
		t.Errorf("zh logs search-hint footer did not switch:\n%s", logs)
	}
	m.logsSearchText = "alpha"
	logs = m.renderLogs()
	if !strings.Contains(logs, "🔍 2/15 个匹配") || strings.Contains(logs, "matches") {
		t.Errorf("zh logs search-matches footer did not switch:\n%s", logs)
	}
	m.logsSearchMode = false
	m.logsSearchText = ""
	logs = m.renderLogs()
	if !strings.Contains(logs, "Esc 返回 • / 搜索") || strings.Contains(logs, "/ to search") {
		t.Errorf("zh logs scroll footer did not switch:\n%s", logs)
	}

	// Update feedback messages.
	m = zh()
	m.Update(exportSuccessMsg{count: 3, filePath: "/tmp/x.csv"})
	if got := m.exportMessage; got != "✅ 已导出 3 项到：/tmp/x.csv" {
		t.Errorf("zh export success message:\n got %q", got)
	}
	m.Update(exportErrorMsg{err: errors.New("disk full")})
	if got := m.exportMessage; got != "❌ 导出失败：disk full" {
		t.Errorf("zh export error message:\n got %q", got)
	}
	m.Update(clipboardResultMsg{text: "web-0"})
	if got := m.exportMessage; got != "✅ 已复制：web-0" {
		t.Errorf("zh clipboard success message:\n got %q", got)
	}
	m.Update(clipboardResultMsg{err: errors.New("no tool")})
	if got := m.exportMessage; got != "❌ 复制失败：no tool" {
		t.Errorf("zh clipboard error message:\n got %q", got)
	}

	// logsError sites.
	m = zh()
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.selectedPod = censusBarePod()
	pressL(m)
	if got := m.logsError; got != "无可用容器" {
		t.Errorf("zh pod-detail logsError:\n got %q", got)
	}
	m = zh()
	m.currentView = ViewJobDetail
	m.detailMode = true
	m.selectedJob = &model.JobData{Name: "web", Namespace: "default"}
	m.clusterData = &model.ClusterData{Pods: []*model.PodData{censusBarePod()}}
	pressL(m)
	if got := m.logsError; got != "无可用容器" {
		t.Errorf("zh job-detail logsError:\n got %q", got)
	}

	// Export errors.
	t.Setenv("HOME", "/sys")
	m = zh()
	if _, err := m.getExportDir(); err == nil {
		t.Fatal("zh getExportDir with HOME=/sys returned no error")
	} else if !strings.HasPrefix(err.Error(), "无法创建导出目录 /sys/.config/k8s-monitor/exports: ") {
		t.Errorf("zh dir-create error prefix:\n got %q", err.Error())
	}
	t.Setenv("HOME", "/dev/null")
	m = zh()
	if _, err := m.getExportDir(); err == nil {
		t.Fatal("zh getExportDir with HOME=/dev/null returned no error")
	} else if !strings.HasPrefix(err.Error(), "导出目录 /dev/null/.config/k8s-monitor/exports 不可写: ") {
		t.Errorf("zh dir-not-writable error prefix:\n got %q", err.Error())
	}
	t.Setenv("HOME", t.TempDir())
	m = zh()
	m.currentView = ViewOverview
	if em, ok := m.exportData(ExportCSV)().(exportErrorMsg); ok {
		if got := em.err.Error(); got != "当前视图不支持导出" {
			t.Errorf("zh view-unsupported error:\n got %q", got)
		}
	}
	for _, tc := range []struct {
		view ViewType
		want string
	}{
		{ViewNodes, "暂无可导出的节点数据"},
		{ViewPods, "暂无可导出的 Pod 数据"},
		{ViewEvents, "暂无可导出的事件数据"},
		{ViewNetwork, "暂无可导出的服务数据"},
	} {
		m := zh()
		m.currentView = tc.view
		m.clusterData = &model.ClusterData{}
		if em, ok := m.exportData(ExportCSV)().(exportErrorMsg); ok {
			if got := em.err.Error(); got != tc.want {
				t.Errorf("zh no-data error for view %d:\n got %q\nwant %q", tc.view, got, tc.want)
			}
		}
	}

	// fetchLogs provider error.
	m = zh()
	m.selectedPod = censusPod()
	m.selectedContainer = "app"
	if cmd := m.fetchLogs(); cmd != nil {
		if lm, ok := cmd().(logsMsg); ok {
			if lm.err == nil || lm.err.Error() != "数据源不支持日志获取" {
				t.Errorf("zh fetchLogs provider error:\n got %v", lm.err)
			}
		}
	}

	// Header byline.
	if got := m.renderHeader(); !strings.Contains(got, "作者 陈普") || strings.Contains(got, " by ") {
		t.Errorf("zh header byline did not switch:\n%s", got)
	}
}

// TestCensusClearExportErrorWrapping is the dedicated %w fidelity probe:
// both keyed filesystem errors must still unwrap to their underlying fs
// error after the batch moved the text into the bundle.
func TestCensusClearExportErrorWrapping(t *testing.T) {
	t.Setenv("HOME", "/dev/null")
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	_, err := m.getExportDir()
	if err == nil {
		t.Fatal("getExportDir with HOME=/dev/null returned no error")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || !errors.Is(err, syscall.ENOTDIR) {
		t.Errorf("keyed export error no longer unwraps: %v (%T)", err, err)
	}
}

package ui

// final_sweep_i18n_test.go — batch 18 (i18n campaign close-out) lockdowns.
//
// Covers the last hardcoded user-facing strings: the top-level view chrome
// (goodbye / width-0 loading), the command-output loading shell, the overview
// loading and empty states, the Superpod prefix in the overview topology
// panel, the kubelet hint triplet (moved verbatim into the bundle for BOTH
// locales — zero visible change), and the logs search bar label.
// Also locks the unified zh schedule term (调度计划, matching
// detail.cronjob.schedule) and adds an AST guard against re-introducing
// hardcoded label literals of the shapes this campaign eliminated.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// superPodNodes returns a fixture with two SuperPods: SP-0 with five nodes
// (exercises the >4-IP "+N" suffix path) and SP-1 with one node.
func superPodNodes() []*model.NodeData {
	return []*model.NodeData{
		{SuperPodID: "SP-0", InternalIP: "10.0.0.1", NPUAllocatable: 8},
		{SuperPodID: "SP-0", InternalIP: "10.0.0.2", NPUAllocatable: 8},
		{SuperPodID: "SP-0", InternalIP: "10.0.0.3", NPUAllocatable: 8},
		{SuperPodID: "SP-0", InternalIP: "10.0.0.4", NPUAllocatable: 8},
		{SuperPodID: "SP-0", InternalIP: "10.0.0.5", NPUAllocatable: 8},
		{SuperPodID: "SP-1", InternalIP: "10.0.1.1", NPUAllocatable: 8},
	}
}

// kubeletHintErrors maps one representative error string per hint kind, in
// the same families detectKubeletHintKind classifies.
func kubeletHintErrors() map[string]kubeletHintKind {
	return map[string]kubeletHintKind{
		"Error: forbidden (nodes/proxy)":                kubeletHintRBAC,
		"x509: certificate signed by unknown authority": kubeletHintTLS,
		"connection refused":                            kubeletHintUnknown,
	}
}

// TestFinalSweepEnGolden pins the English bytes of every site touched by
// batch 18 — they must stay identical to the pre-batch hardcoded strings.
func TestFinalSweepEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	// Top-level chrome (view.go).
	if got := func() string { en.quitting = true; defer func() { en.quitting = false }(); return en.View() }(); got != "Goodbye!\n" {
		t.Errorf("en quit view:\n got %q\nwant %q", got, "Goodbye!\n")
	}
	if got := en.View(); got != "Loading..." {
		t.Errorf("en width-0 shell:\n got %q\nwant %q", got, "Loading...")
	}

	// Command output loading shell (command_output.go).
	if got := en.renderCommandOutput(); got != "Loading..." {
		t.Errorf("en command output loading:\n got %q\nwant %q", got, "Loading...")
	}

	// Overview loading / empty states.
	if got := en.renderOverview(); got != "Loading cluster data..." {
		t.Errorf("en overview no-data:\n got %q\nwant %q", got, "Loading cluster data...")
	}
	en.clusterData = &model.ClusterData{}
	if got := en.renderOverview(); got != "No cluster summary available" {
		t.Errorf("en overview no-summary:\n got %q\nwant %q", got, "No cluster summary available")
	}

	// Superpod prefix line in the overview topology panel (full bytes,
	// including the border box and padding).
	en.clusterData = &model.ClusterData{Nodes: superPodNodes()}
	wantLines := []string{
		"│    Superpod SP-0: 10.0.0.1, 10.0.0.2, 10.0.0.3, 10.0.0.4 +1  (5 nodes × 8 NPU = 40 NPU)                           │",
		"│    Superpod SP-1: 10.0.1.1  (1 nodes × 8 NPU = 8 NPU)                                                             │",
	}
	for _, want := range wantLines {
		if !strings.Contains(en.renderSuperPodTopology(), want) {
			t.Errorf("en superpod golden line not found:\nwant %q", want)
		}
	}

	// Kubelet hints: detection mapping unchanged plus byte-exact values.
	for errMsg, wantKind := range kubeletHintErrors() {
		if got := detectKubeletHintKind(errMsg); got != wantKind {
			t.Errorf("detectKubeletHintKind(%q) = %d, want %d", errMsg, got, wantKind)
		}
	}
	for kind, want := range map[kubeletHintKind]string{
		kubeletHintRBAC:    "Check RBAC: kubectl auth can-i get nodes/proxy",
		kubeletHintTLS:     "Use --insecure-kubelet for TLS issues (test clusters only)",
		kubeletHintUnknown: "Try --insecure-kubelet",
	} {
		if got := en.kubeletHint(kind); got != want {
			t.Errorf("en kubeletHint(%d):\n got %q\nwant %q", kind, got, want)
		}
	}

	// Logs search bar: the trailing space after "Search:" stays part of the
	// en bundle value so en rendering is byte-identical to the old
	// StyleKey.Render("Search: ").
	en.selectedPod = &model.PodData{Name: "p1", Namespace: "default"}
	en.logsSearchMode = true
	wantLogs := " 📜 Logs: default/p1   Container: \nSearch: _\n\nLoading logs..."
	if got := en.renderLogs(); got != wantLogs {
		t.Errorf("en logs search bar:\n got %q\nwant %q", got, wantLogs)
	}
}

// TestFinalSweepZhSwitch verifies the Chinese side switches at every batch-18
// site. The kubelet hints are asserted byte-exact in BOTH locales: the move
// into the bundle was a pure migration with zero visible change.
func TestFinalSweepZhSwitch(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	if got := func() string { zh.quitting = true; defer func() { zh.quitting = false }(); return zh.View() }(); got != "再见！\n" {
		t.Errorf("zh quit view:\n got %q\nwant %q", got, "再见！\n")
	}
	if got := zh.View(); got != "加载中..." {
		t.Errorf("zh width-0 shell:\n got %q\nwant %q", got, "加载中...")
	}

	if got := zh.renderCommandOutput(); got != "加载中..." {
		t.Errorf("zh command output loading:\n got %q\nwant %q", got, "加载中...")
	}

	if got := zh.renderOverview(); got != "正在加载集群数据..." {
		t.Errorf("zh overview no-data:\n got %q\nwant %q", got, "正在加载集群数据...")
	}
	zh.clusterData = &model.ClusterData{}
	if got := zh.renderOverview(); got != "暂无集群摘要" {
		t.Errorf("zh overview no-summary:\n got %q\nwant %q", got, "暂无集群摘要")
	}

	// Superpod prefix switches and no en term leaks (the panel title is
	// already zh via topology.superpod.title; the per-row prefix was the
	// last hardcoded piece).
	zh.clusterData = &model.ClusterData{Nodes: superPodNodes()}
	topology := zh.renderSuperPodTopology()
	for _, noun := range []string{"超节点 SP-0: 10.0.0.1", "超节点 SP-1: 10.0.1.1"} {
		if !strings.Contains(topology, noun) {
			t.Errorf("zh superpod line %q does not contain %q", topology, noun)
		}
	}
	for _, bad := range []string{"Superpod ", "SuperPod "} {
		if strings.Contains(topology, bad) {
			t.Errorf("zh superpod topology %q still contains %q", topology, bad)
		}
	}

	// Kubelet hints byte-exact in both locales (pure migration), and the
	// locales must actually differ for the RBAC/TLS kinds.
	for kind, want := range map[kubeletHintKind]string{
		kubeletHintRBAC:    "检查 RBAC：kubectl auth can-i get nodes/proxy",
		kubeletHintTLS:     "使用 --insecure-kubelet（仅限测试环境）",
		kubeletHintUnknown: "尝试 --insecure-kubelet",
	} {
		if got := zh.kubeletHint(kind); got != want {
			t.Errorf("zh kubeletHint(%d):\n got %q\nwant %q", kind, got, want)
		}
	}
	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	for _, kind := range []kubeletHintKind{kubeletHintRBAC, kubeletHintTLS} {
		if en.kubeletHint(kind) == zh.kubeletHint(kind) {
			t.Errorf("kubeletHint(%d): zh equals en %q — locale did not switch", kind, zh.kubeletHint(kind))
		}
	}

	// Hint in context: the metrics panel carries the zh hint verbatim.
	zh.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{
		TotalNodes: 2, KubeletError: "Error: forbidden (nodes/proxy)",
	}}
	panel := zh.renderClusterLoadCompact(zh.clusterData.Summary)
	for _, want := range []string{
		"检查 RBAC：kubectl auth can-i get nodes/proxy",
		"💡 检查 RBAC：kubectl auth can-i get nodes/proxy",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("zh metrics panel %q does not contain %q", panel, want)
		}
	}

	// Logs search bar switches; en label must not leak.
	zh.selectedPod = &model.PodData{Name: "p1", Namespace: "default"}
	zh.logsSearchMode = true
	logs := zh.renderLogs()
	if !strings.Contains(logs, "搜索：_") {
		t.Errorf("zh logs search bar %q does not contain %q", logs, "搜索：_")
	}
	if strings.Contains(logs, "Search:") {
		t.Errorf("zh logs search bar %q still contains the en label", logs)
	}
}

// TestFinalSweepScheduleColumnZh locks the unified zh schedule term. The old
// cell (调度) is asserted via its exact padded form so it cannot be masked by
// the 调度 substring inside 调度计划 or inside the 上次调度 header (whose padding
// is 14 spaces, never the old cell's 18).
func TestFinalSweepScheduleColumnZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	zh.clusterData = &model.ClusterData{CronJobs: []*model.CronJobData{
		{Name: "cj1", Namespace: "default", Schedule: "*/5 * * * *", Active: 2},
	}}
	rows, _ := zh.renderCronJobsList(0)
	header := rows[2]

	if !strings.Contains(header, padRight("调度计划", 20)) {
		t.Errorf("zh cronjobs header %q does not contain the padded 调度计划 cell", header)
	}
	if strings.Contains(header, padRight("调度", 20)) {
		t.Errorf("zh cronjobs header %q still contains the old padded 调度 cell", header)
	}

	// Full-row byte golden for the zh header (schedule cell renamed, widths
	// unchanged: 调度计划 is 8 display columns, same as SCHEDULE).
	wantHeader := "名称                            命名空间         调度计划              暂停        活跃        上次调度            "
	if header != wantHeader {
		t.Errorf("zh cronjobs header:\n got %q\nwant %q", header, wantHeader)
	}

	// en keeps SCHEDULE and its whole header row byte-identical.
	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.clusterData = &model.ClusterData{CronJobs: zh.clusterData.CronJobs}
	enRows, _ := en.renderCronJobsList(0)
	wantEnHeader := "NAME                            NAMESPACE        SCHEDULE              SUSPEND     ACTIVE      LAST SCHEDULE       "
	if got := enRows[2]; got != wantEnHeader {
		t.Errorf("en cronjobs header:\n got %q\nwant %q", got, wantEnHeader)
	}
}

// TestFinalSweepGuardNoHardcodedUILabels is the census ratchet. It parses
// every non-test file in the ui package and rejects two shapes the i18n
// campaign eliminated:
//
//	A) a string literal containing ASCII letters passed directly to a
//	   .Render(...) call (the "StyleKey.Render(\"Search: \")" shape);
//	B) a bare `return <string literal with letters>` statement (the
//	   "return \"Goodbye!\"" shape; the kubelet hint helpers used it too).
//
// Allowlist entries are frozen and must carry a reason. Four entries are
// KNOWN RESIDUALS reported by the batch-18 census (action_menu.go menu
// labels, overview cpu/memory panel headers) — they are scheduled for their
// own batch and must not grow.
func TestFinalSweepGuardNoHardcodedUILabels(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the ui package directory")
	}
	dir := filepath.Dir(thisFile)

	hasLetters := regexp.MustCompile(`[A-Za-z]`).MatchString

	// Pattern A allowlist: value -> reason.
	allowRender := map[string]string{
		"True":      "k8s boolean value (suspend column), untranslated by convention",
		"False":     "k8s boolean value (suspend column), untranslated by convention",
		"0 B/s":     "zero-rate placeholder (unit, not a label)",
		"0B":        "zero-size placeholder (unit, not a label)",
		"  🧮 NPU: ": "technical term prefix, locale-invariant",
		// Known residuals from the batch-18 census — pending their own batch.
		"🎯 Quick Actions": "KNOWN RESIDUAL (action menu title) — census batch 18",
		"  ↑/↓ Navigate • Enter Select • ESC Cancel": "KNOWN RESIDUAL (action menu help) — census batch 18",
		"[⚡ CPU]":    "KNOWN RESIDUAL (overview panel header) — census batch 18",
		"[🧠 Memory]": "KNOWN RESIDUAL (overview panel header) — census batch 18",
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("cannot list ui package files in %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	for _, fn := range files {
		if strings.HasSuffix(fn, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, fn, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", fn, err)
		}
		base := filepath.Base(fn)
		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				sel, isSel := node.Fun.(*ast.SelectorExpr)
				if !isSel || sel.Sel.Name != "Render" {
					return true
				}
				for _, arg := range node.Args {
					lit, isLit := arg.(*ast.BasicLit)
					if !isLit || lit.Kind != token.STRING || !hasLetters(lit.Value) {
						continue
					}
					value := strings.Trim(lit.Value, `"`)
					reason, allowed := allowRender[value]
					if !allowed {
						t.Errorf("%s:%d: hardcoded label passed to Render: %s (use m.T with a bundle key, or extend the census allowlist with a reason)", base, fset.Position(lit.Pos()).Line, lit.Value)
						continue
					}
					_ = reason
				}
			case *ast.ReturnStmt:
				if len(node.Results) != 1 {
					return true
				}
				lit, isLit := node.Results[0].(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING || !hasLetters(lit.Value) {
					return true
				}
				t.Errorf("%s:%d: bare return of a string literal with letters: %s (use m.T with a bundle key)", base, fset.Position(lit.Pos()).Line, lit.Value)
			}
			return true
		})
	}
}

// TestQueuesScrollIndicatorKeyed locks the queues list scroll indicator onto
// the shared detail.scroll_indicator_viewing key (batch-18 verify finding:
// the census missed this site; the string is byte-identical to the pvc/pv
// variant keyed in batch 16, so the key is reused, not duplicated).
// maxVisible is max(height-10, 5); height 15 forces a 1-5 window over the
// full detail, whose line count exceeds 5 for any queue.
func TestQueuesScrollIndicatorKeyed(t *testing.T) {
	forcedAsciiProfile(t)

	mk := func(locale string) *Model {
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
		m.selectedQueue = &model.QueueData{Name: "q-a", State: "Open", Weight: 1}
		m.height = 15
		return m
	}

	en := mk("en").renderQueueDetail()
	if !strings.Contains(en, "(viewing 1-5 of ") || !strings.HasSuffix(en, "to scroll)") {
		t.Errorf("en queues scroll indicator mismatch:\n%s", en)
	}

	zh := mk("zh").renderQueueDetail()
	if !strings.Contains(zh, "（查看第 1-5 行，共 ") || !strings.HasSuffix(zh, "滚动）") {
		t.Errorf("zh queues scroll indicator mismatch:\n%s", zh)
	}
	if strings.Contains(zh, "(viewing") {
		t.Errorf("zh queues render still contains the English scroll hint:\n%s", zh)
	}
}

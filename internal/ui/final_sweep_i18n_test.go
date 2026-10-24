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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// verbRe matches one fmt verb with its flags/width/precision, e.g. "%-10.2f",
// "%*d", "%%". Used to strip verbs from format strings before checking for
// hardcoded words (pattern C).
var verbRe = regexp.MustCompile(`%[-+ #0]*(\*|\d+)?(\.(\*|\d+)?)?[a-zA-Z%]`)

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
// every non-test file in the ui package and rejects three shapes the i18n
// campaign eliminated:
//
//	A) a string literal containing ASCII letters passed directly to a
//	   .Render(...) call (the "StyleKey.Render(\"Search: \")" shape);
//	B) a bare `return <string literal with letters>` statement (the
//	   "return \"Goodbye!\"" shape; the kubelet hint helpers used it too);
//	C) a fmt.Sprintf/Fprintf/Errorf first argument whose letters survive
//	   stripping the fmt verbs — i.e. a format string carrying hardcoded
//	   words, not just verbs and separators (the "Sched: %s" shape).
//
// Pattern C auto-allows verb-only formats ("  %s: %s", "%.0f%% (%d/%d)"):
// after verb stripping they leave no letters, so its allowlist stays
// value-class only. Every allowlist entry is frozen and carries a reason.
func TestFinalSweepGuardNoHardcodedUILabels(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the ui package directory")
	}
	dir := filepath.Dir(thisFile)

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("cannot list ui package files in %s: %v", dir, err)
	}

	for _, v := range censusScan(t, files) {
		t.Errorf("%s", v)
	}
}

// unquoteLiteral returns the interpreted value of a string literal, falling
// back to trimming the quotes when the source is not a valid Go literal.
func unquoteLiteral(lit *ast.BasicLit) string {
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return strings.Trim(lit.Value, `"`)
	}
	return value
}

// stringLiterals returns every string-literal operand of expr, in source
// order, together with its position. It descends into +-concatenations and
// parenthesised groups so that an operand split across literals — the
// "lit" + expr shape that used to be invisible to the census — is reported
// as the literals it is made of. Non-literal operands (identifiers, calls)
// yield nothing: only what the compiler would splice into the value counts.
func stringLiterals(expr ast.Expr) []*ast.BasicLit {
	var lits []*ast.BasicLit
	ast.Inspect(expr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			// Add is string concatenation only when both operands are
			// strings; a numeric expression's literals are not UI copy and
			// are not reported (their Kind is not STRING anyway).
			return node.Op == token.ADD
		case *ast.ParenExpr:
			return true
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				lits = append(lits, node)
			}
			return false
		}
		return false
	})
	return lits
}

// joinLiterals unquotes the collected literals and joins them into the single
// value the assembled expression evaluates to for its literal parts. Only the
// literals are joined, so the result is what a reader of the site sees as
// words: the non-literal operands contribute their runtime value, not copy.
func joinLiterals(lits []*ast.BasicLit) string {
	var b strings.Builder
	for _, lit := range lits {
		b.WriteString(unquoteLiteral(lit))
	}
	return b.String()
}

// censusScan parses the given files and returns one violation string per
// hardcoded-label site (patterns A/B/C above; test files are skipped).
// It is a standalone function so the anti-blind-positive probe can run it
// against injected files without touching the package directory.
//
// Every pattern reads the *literal parts* of its expression rather than a
// bare literal node, so a concatenation cannot hide copy behind an operand
// boundary ("Hard" + "coded" is the same label as "Hardcoded"; "%.1f" + "G"
// is the same format as "%.1fG"). Format strings are judged after joining,
// because the verbs and the words around them may sit in different operands.
func censusScan(t *testing.T, files []string) []string {
	t.Helper()

	hasLetters := regexp.MustCompile(`[A-Za-z]`).MatchString

	// Pattern A allowlist: value -> reason.
	allowRender := map[string]string{
		"True":  "k8s boolean value (suspend column), untranslated by convention",
		"False": "k8s boolean value (suspend column), untranslated by convention",
		"0 B/s": "zero-rate placeholder (unit, not a label)",
		"0B":    "zero-size placeholder (unit, not a label)",
	}

	// Pattern C allowlist: format literal -> reason. Entries are value-class:
	// their letters survive verb stripping because they name files, units or
	// technical terms — never UI copy. Frozen at batch 19 (census clear-out).
	allowFmt := map[string]string{
		"unsupported operation": "sentinel error value; commandOutputMsg.err is dropped by Update (never rendered) — the title/content carry the user copy",

		// Export filename prefixes — product artifacts, not UI copy.
		"k8s-nodes-%s":    "export filename prefix",
		"k8s-pods-%s":     "export filename prefix",
		"k8s-events-%s":   "export filename prefix",
		"k8s-services-%s": "export filename prefix",

		// NPU / CPU technical terms and allocation arithmetic.
		"%d NPU":                            "NPU count, technical term",
		"%s: %s (%d NPU)":                   "NPU count annotation, technical term",
		"  %s %s: %s  (%d%s × %d NPU = %s)": "superpod NPU arithmetic line, technical term",
		"%dx%d=%d":                          "NPU allocation multiplication rendering (task×replica=result)",
		"        CPU: %s":                   "CPU technical-term label, locale-invariant by convention (columns.cpu)",

		// k8s resource request/limit annotations (technical key=value form).
		" req=%s":    "k8s request annotation, technical form",
		" limit=%s":  "k8s limit annotation, technical form",
		" (req: %s)": "k8s request annotation, technical form",

		// Scroll-indicator frame: the verbs carry keyed labels, the surviving
		// letters are key names (PgUp/PgDn).
		"\n[%s] (↑/↓ %s, PgUp/PgDn %s)": "scroll hint frame; key names only, labels come from keyed verbs",

		// Unit suffixes — value formatting, not labels.
		"%.1fG":                            "size unit (G)",
		"%.1fGi":                           "size unit (Gi)",
		"%.1fK":                            "size unit (K)",
		"%.1fKB":                           "size unit (KB)",
		"%.1fKi":                           "size unit (Ki)",
		"%.1fMB":                           "size unit (MB)",
		"%.1fMi":                           "size unit (Mi)",
		"%.1fT":                            "size unit (T)",
		"%.1fTi":                           "size unit (Ti)",
		"%.2fG":                            "size unit (G)",
		"%.2fGB":                           "size unit (GB)",
		"%.2fM":                            "size unit (M)",
		"%.2fTB":                           "size unit (TB)",
		"%.0fK":                            "size unit (K)",
		"%.0fM":                            "size unit (M)",
		"%d B":                             "size unit (B)",
		"%dB":                              "size unit (B)",
		"%d/%d MB":                         "size unit (MB)",
		"%d/%dMB (%.0f%%)":                 "size unit with percentage (MB)",
		"%.1f/%.0fG(%d%%)":                 "size unit with percentage (G)",
		"    %s: %.1f / %.1f GiB (%.1f%%)": "size unit with percentage (GiB)",
		"%.1f %ciB":                        "size unit built from a %c SI prefix + iB",
		"%.1f KB/s":                        "rate unit (KB/s)",
		"%.0f KB/s":                        "rate unit (KB/s)",
		"%.1f MB/s":                        "rate unit (MB/s)",
		"%.2f MB/s":                        "rate unit (MB/s)",
		"%.2f GB/s":                        "rate unit (GB/s)",
		"%.1fW":                            "power unit (W)",
		"%dW":                              "power unit (W)",
		"    %s: %dW":                      "power unit (W)",
		"%.2fV":                            "voltage unit (V)",
		"%d°C":                             "temperature unit (°C)",
		"    %s: %d W   %s: %d °C":         "power/temperature units (W, °C)",
		"%dd":                              "duration unit (days)",
		"%dh":                              "duration unit (hours)",
		"%dh%dm":                           "duration units (h/m)",
		"%dm":                              "duration unit (minutes)",
		"%dm%ds":                           "duration units (m/s)",
		"%ds":                              "duration unit (seconds)",
		"%dMHz":                            "frequency unit (MHz)",
	}

	var violations []string
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
				if !isSel {
					return true
				}
				switch sel.Sel.Name {
				case "Render":
					for _, arg := range node.Args {
						for _, lit := range stringLiterals(arg) {
							// Judge the interpreted value: the raw source of
							// "\n" carries an 'n', which would otherwise be
							// reported as a letter-bearing label.
							value := unquoteLiteral(lit)
							if !hasLetters(value) {
								continue
							}
							if _, allowed := allowRender[value]; !allowed {
								violations = append(violations, fmt.Sprintf("%s:%d: hardcoded label passed to Render: %q (use m.T with a bundle key, or extend the census allowlist with a reason)", base, fset.Position(lit.Pos()).Line, value))
							}
						}
					}
				case "Sprintf", "Fprintf", "Errorf":
					pkg, isPkg := sel.X.(*ast.Ident)
					if !isPkg || pkg.Name != "fmt" {
						return true
					}
					// Sprintf/Errorf take the format first; Fprintf takes it
					// after the writer.
					fmtIdx := 0
					if sel.Sel.Name == "Fprintf" {
						fmtIdx = 1
					}
					if len(node.Args) <= fmtIdx {
						return true
					}
					lits := stringLiterals(node.Args[fmtIdx])
					if len(lits) == 0 {
						return true
					}
					// Work on the interpreted value so escape sequences
					// (\n, \t, …) cannot masquerade as letters. The joined
					// value is what the verbs and words are judged against:
					// "%.1f" + "G" concatenates to the same allowed "%.1fG".
					value := joinLiterals(lits)
					// Strip fmt verbs: what remains must be free of letters,
					// else the format string carries hardcoded words.
					nonVerb := verbRe.ReplaceAllString(value, "")
					if !hasLetters(nonVerb) {
						return true
					}
					if _, allowed := allowFmt[value]; !allowed {
						violations = append(violations, fmt.Sprintf("%s:%d: hardcoded words in fmt format string: %q (use m.T/m.TF with a bundle key, or extend the census allowlist with a reason)", base, fset.Position(lits[0].Pos()).Line, value))
					}
				}
			case *ast.ReturnStmt:
				if len(node.Results) != 1 {
					return true
				}
				lits := stringLiterals(node.Results[0])
				if len(lits) == 0 {
					return true
				}
				value := joinLiterals(lits)
				if !hasLetters(value) {
					return true
				}
				violations = append(violations, fmt.Sprintf("%s:%d: bare return of a string literal with letters: %q (use m.T with a bundle key)", base, fset.Position(lits[0].Pos()).Line, value))
			}
			return true
		})
	}
	return violations
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

// TestCensusGuardPatternsFire is the anti-blind-positive probe: the census
// scanner must report each of its three shapes on injected files. The probe
// files live in a temp dir (never the package dir) and are deleted by the
// temp-dir cleanup.
func TestCensusGuardPatternsFire(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		// Pattern C: fmt format string carrying hardcoded words. The file is
		// only parsed (never compiled), so the fmt import can be omitted.
		writeProbe(t, dir, "probe_fmt.go", "package ui\n\nfunc probeFmt() string {\n\treturn fmt.Sprintf(\"Hardcoded label %d\", 1)\n}\n"),
		// Pattern A: letter-bearing literal passed straight to Render.
		writeProbe(t, dir, "probe_render.go", "package ui\n\ntype probeStyle struct{}\n\nfunc (probeStyle) Render(s string) string { return s }\n\nfunc probeRender() string {\n\tvar st probeStyle\n\treturn st.Render(\"Hardcoded X\")\n}\n"),
		// Pattern B: bare return of a letter-bearing string literal.
		writeProbe(t, dir, "probe_return.go", "package ui\n\nfunc probeReturn() string {\n\treturn \"Hardcoded Y\"\n}\n"),
	}

	violations := censusScan(t, files)
	if len(violations) != 3 {
		t.Fatalf("censusScan reported %d violations on the probe files, want exactly 3 (one per pattern):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
	shapes := map[string]bool{
		"Hardcoded label %d": false, // pattern C
		"Hardcoded X":        false, // pattern A
		"Hardcoded Y":        false, // pattern B
	}
	for _, v := range violations {
		for shape := range shapes {
			if strings.Contains(v, shape) {
				shapes[shape] = true
			}
		}
	}
	for shape, seen := range shapes {
		if !seen {
			t.Errorf("censusScan missed the %q probe shape:\n%s", shape, strings.Join(violations, "\n"))
		}
	}
}

// TestCensusGuardConcatenationFire is the batch-26 anti-blind-positive probe
// for the concatenation blind spot: each pattern re-fired with its label split
// across a +-concatenation, so no single *ast.BasicLit carries the copy. The
// scanner must descend into the operands and report the same shapes it reports
// for the unsplit literals. It also pins the join boundary in the other
// direction: "%.1f" + "G" assembles into the *allowed* "%.1fG" and stays silent,
// which only holds if the format string is judged after joining rather than
// operand by operand.
func TestCensusGuardConcatenationFire(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		// Pattern C, concatenated: the words straddle the operator.
		writeProbe(t, dir, "probe_fmt_concat.go", "package ui\n\nfunc probeFmtConcat(n int) string {\n\treturn fmt.Sprintf(\"Join\"+\"ed label %d\", n)\n}\n"),
		// Pattern A, concatenated: neither operand is the whole label.
		writeProbe(t, dir, "probe_render_concat.go", "package ui\n\ntype probeStyle struct{}\n\nfunc (probeStyle) Render(s string) string { return s }\n\nfunc probeRenderConcat() string {\n\tvar st probeStyle\n\treturn st.Render(\"Hard\" + \"coded\")\n}\n"),
		// Pattern B, concatenated.
		writeProbe(t, dir, "probe_return_concat.go", "package ui\n\nfunc probeReturnConcat() string {\n\treturn \"Hard\" + \"coded\"\n}\n"),
		// Pattern B split by parentheses rather than directly: the descent must
		// survive the ParenExpr around a grouped operand.
		writeProbe(t, dir, "probe_return_paren.go", "package ui\n\nfunc probeReturnParen() string {\n\treturn \"Hard\" + (\"coded\")\n}\n"),
		// Negative control: verb + unit split across operands, assembling into
		// the allowlisted "%.1fG". Must not be reported.
		writeProbe(t, dir, "probe_fmt_concat_allowed.go", "package ui\n\nfunc probeFmtConcatAllowed(v float64) string {\n\treturn fmt.Sprintf(\"%.1f\"+\"G\", v)\n}\n"),
	}

	violations := censusScan(t, files)
	// Five probe files yield five violations: the concatenated Render probe
	// reports both of its operands (each alone is already a hardcoded
	// fragment), and the two concatenated returns report their joined values.
	// The allowlisted "%.1f"+"G" contributes nothing.
	if len(violations) != 5 {
		t.Fatalf("censusScan reported %d violations on the concatenation probes, want exactly 5:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
	shapes := map[string]bool{
		"Joined label %d": false, // pattern C, concatenated
		`"Hard"`:          false, // pattern A, concatenated (first operand)
		`"coded"`:         false, // pattern A, concatenated (second operand)
		`"Hardcoded"`:     false, // pattern B, concatenated (joined operands)
	}
	for _, v := range violations {
		for shape := range shapes {
			if strings.Contains(v, shape) {
				shapes[shape] = true
			}
		}
	}
	for shape, seen := range shapes {
		if !seen {
			t.Errorf("censusScan missed the %q concatenation probe shape:\n%s", shape, strings.Join(violations, "\n"))
		}
	}
	for _, v := range violations {
		if strings.Contains(v, "probe_fmt_concat_allowed.go") {
			t.Errorf("the allowlisted post-join format was reported: %s", v)
		}
	}
}

func writeProbe(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

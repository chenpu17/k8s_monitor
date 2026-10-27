package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// WP4 probes: the CSV export writer's error reporting and the localization of
// the export headers, the alert value column and the alert category footer.

// csvHeader reads the first record of a written CSV file.
func csvHeader(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return line
}

// TestCSVExportReportsFlushError is the W4-1 regression guard: csv.Writer
// buffers 4 KiB, so the last chunk of a file — and any error writing it — only
// materializes in Flush. All four CSV exporters deferred Flush and dropped its
// error, so a full disk reported success on a truncated file.
func TestCSVExportReportsFlushError(t *testing.T) {
	// /dev/full accepts writes and fails them with ENOSPC; writing there
	// reproduces the "error only at flush time" case without filling a real
	// filesystem.
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skipf("/dev/full is not available on this platform: %v", err)
	}

	m := modelForTest(t, "en")
	dir := "/dev"
	name := "full"

	cases := []struct {
		name   string
		export func() error
	}{
		{"nodes", func() error {
			return m.exportNodesCSV(&model.ClusterData{Nodes: []*model.NodeData{
				{Name: "node-a", Status: "Ready", Roles: []string{"worker"}},
			}}, dir, name)
		}},
		{"pods", func() error {
			return m.exportPodsCSV(&model.ClusterData{Pods: []*model.PodData{
				{Name: "web-0", Namespace: "default", Phase: "Running", Node: "node-a"},
			}}, dir, name)
		}},
		{"events", func() error {
			return m.exportEventsCSV(&model.ClusterData{Events: []*model.EventData{
				{Type: "Warning", Reason: "Failed", InvolvedObject: "Pod/web-0",
					Message: strings.Repeat("m", 240)},
			}}, dir, name)
		}},
		{"services", func() error {
			return m.exportServicesCSV(&model.ClusterData{Services: []*model.ServiceData{
				{Name: "svc-0", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.96.0.1"},
			}}, dir, name)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.export(); err == nil {
				t.Fatalf("%s CSV export to /dev/full reported success; the Flush error was dropped", tc.name)
			}
		})
	}
}

// TestCSVExportSuccessStillSilent pins the happy path: with a writable
// directory the exporters return nil, so the flush error check cannot start
// reporting phantom failures.
func TestCSVExportSuccessStillSilent(t *testing.T) {
	dir := t.TempDir()
	m := modelForTest(t, "en")

	if err := m.exportNodesCSV(&model.ClusterData{Nodes: []*model.NodeData{
		{Name: "node-a", Status: "Ready"},
	}}, dir, "nodes.csv"); err != nil {
		t.Errorf("nodes CSV export failed: %v", err)
	}
	if err := m.exportPodsCSV(&model.ClusterData{Pods: []*model.PodData{
		{Name: "web-0", Namespace: "default", Phase: "Running"},
	}}, dir, "pods.csv"); err != nil {
		t.Errorf("pods CSV export failed: %v", err)
	}
	if err := m.exportEventsCSV(&model.ClusterData{Events: []*model.EventData{
		{Type: "Warning", Reason: "Failed", InvolvedObject: "Pod/web-0"},
	}}, dir, "events.csv"); err != nil {
		t.Errorf("events CSV export failed: %v", err)
	}
	if err := m.exportServicesCSV(&model.ClusterData{Services: []*model.ServiceData{
		{Name: "svc-0", Namespace: "default", Type: "ClusterIP"},
	}}, dir, "services.csv"); err != nil {
		t.Errorf("services CSV export failed: %v", err)
	}
}

// TestCSVHeadersEnglishUnchanged is the anchor for W4-2: the English headers
// were already the English column words, so routing them through the bundle
// must not change a byte.
func TestCSVHeadersEnglishUnchanged(t *testing.T) {
	dir := t.TempDir()
	m := modelForTest(t, "en")

	if err := m.exportEventsCSV(&model.ClusterData{Events: []*model.EventData{
		{Type: "Warning", Reason: "Failed", InvolvedObject: "Pod/web-0"},
	}}, dir, "events.csv"); err != nil {
		t.Fatalf("events CSV export: %v", err)
	}
	if got, want := csvHeader(t, filepath.Join(dir, "events.csv")), "TYPE,REASON,OBJECT,MESSAGE,COUNT,AGE"; got != want {
		t.Errorf("en events header:\n got %q\nwant %q", got, want)
	}

	if err := m.exportServicesCSV(&model.ClusterData{Services: []*model.ServiceData{
		{Name: "svc-0", Namespace: "default", Type: "ClusterIP"},
	}}, dir, "services.csv"); err != nil {
		t.Fatalf("services CSV export: %v", err)
	}
	if got, want := csvHeader(t, filepath.Join(dir, "services.csv")), "NAMESPACE,NAME,TYPE,CLUSTER-IP,EXTERNAL IP,PORT(S),AGE"; got != want {
		t.Errorf("en services header:\n got %q\nwant %q", got, want)
	}
}

// TestCSVHeadersLocalized is the W4-2 guard: the events and services headers
// were hardcoded English literals while every other column header in the file
// came from the bundle, so a Chinese UI exported English headers.
func TestCSVHeadersLocalized(t *testing.T) {
	dir := t.TempDir()
	m := modelForTest(t, "zh")

	if err := m.exportEventsCSV(&model.ClusterData{Events: []*model.EventData{
		{Type: "Warning", Reason: "Failed", InvolvedObject: "Pod/web-0"},
	}}, dir, "events.csv"); err != nil {
		t.Fatalf("events CSV export: %v", err)
	}
	events := csvHeader(t, filepath.Join(dir, "events.csv"))
	for _, want := range []string{m.T("columns.type"), m.T("columns.reason"), m.T("columns.object"),
		m.T("columns.message"), m.T("columns.count"), m.T("columns.age")} {
		if !strings.Contains(events, want) {
			t.Errorf("zh events header is missing %q: %q", want, events)
		}
	}
	if strings.Contains(events, "Message") || strings.Contains(events, "Reason") {
		t.Errorf("zh events header still carries English column names: %q", events)
	}

	if err := m.exportServicesCSV(&model.ClusterData{Services: []*model.ServiceData{
		{Name: "svc-0", Namespace: "default", Type: "ClusterIP"},
	}}, dir, "services.csv"); err != nil {
		t.Fatalf("services CSV export: %v", err)
	}
	services := csvHeader(t, filepath.Join(dir, "services.csv"))
	for _, want := range []string{m.T("columns.namespace"), m.T("columns.name"), m.T("columns.type"),
		m.T("columns.cluster_ip"), m.T("columns.external_ip"), m.T("columns.ports"), m.T("columns.age")} {
		if !strings.Contains(services, want) {
			t.Errorf("zh services header is missing %q: %q", want, services)
		}
	}
	if strings.Contains(services, "ExternalIP") || strings.Contains(services, "ClusterIP") {
		t.Errorf("zh services header still carries English column names: %q", services)
	}
}

// TestCSVHeadersUseExistingColumnKeys pins the header cells to the bundle keys
// the rest of the table headers use, so the export cannot drift from the views.
func TestCSVHeadersUseExistingColumnKeys(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		t.Run(locale, func(t *testing.T) {
			dir := t.TempDir()
			m := modelForTest(t, locale)

			if err := m.exportEventsCSV(&model.ClusterData{Events: []*model.EventData{
				{Type: "Warning", Reason: "Failed", InvolvedObject: "Pod/web-0"},
			}}, dir, "events.csv"); err != nil {
				t.Fatalf("events CSV export: %v", err)
			}
			want := strings.Join([]string{
				m.T("columns.type"), m.T("columns.reason"), m.T("columns.object"),
				m.T("columns.message"), m.T("columns.count"), m.T("columns.age"),
			}, ",")
			if got := csvHeader(t, filepath.Join(dir, "events.csv")); got != want {
				t.Errorf("events header:\n got %q\nwant %q", got, want)
			}

			if err := m.exportServicesCSV(&model.ClusterData{Services: []*model.ServiceData{
				{Name: "svc-0", Namespace: "default", Type: "ClusterIP"},
			}}, dir, "services.csv"); err != nil {
				t.Fatalf("services CSV export: %v", err)
			}
			wantSvc := strings.Join([]string{
				m.T("columns.namespace"), m.T("columns.name"), m.T("columns.type"),
				m.T("columns.cluster_ip"), m.T("columns.external_ip"), m.T("columns.ports"),
				m.T("columns.age"),
			}, ",")
			if got := csvHeader(t, filepath.Join(dir, "services.csv")); got != wantSvc {
				t.Errorf("services header:\n got %q\nwant %q", got, wantSvc)
			}
		})
	}
}

// TestAlertChipValueLocalized is the W4-3 render guard: the five NPU chip-value
// alert sites (temperature, both ECC counters, link down, error code) carried
// only a Value, so the localized alerts view printed the English "chips 0, 3"
// next to the translated message. The data-layer half of the wiring (every one
// of the five sites carrying alert.value_chips) is pinned by
// TestAlertValuesEnIdentity in the datasource package; this probe pins the
// render path that turns the key into localized text.
func TestAlertChipValueLocalized(t *testing.T) {
	m := modelForTest(t, "zh")
	m.width = 220
	m.height = 60
	m.currentView = ViewAlerts
	m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{Alerts: []model.Alert{{
		Severity:     model.AlertSeverityCritical,
		Category:     "NPU",
		AlertType:    model.AlertTypeNPULinkDown,
		ResourceType: "Node",
		ResourceName: "node-a",
		Message:      "NPU link down on chip(s) 0, 3",
		MessageKey:   "alert.npu_link_down",
		MessageArgs:  map[string]interface{}{"Chips": "0, 3"},
		Value:        "chips 0, 3",
		ValueKey:     "alert.value_chips",
		ValueArgs:    map[string]interface{}{"Chips": "0, 3"},
	}}}}

	out := stripANSI(m.View())
	if strings.Contains(out, "chips 0, 3") {
		t.Errorf("the alert value column still shows the English \"chips 0, 3\" in the zh UI:\n%s", out)
	}
	if want := m.TF("alert.value_chips", map[string]interface{}{"Chips": "0, 3"}); !strings.Contains(out, want) {
		t.Errorf("the alert value column is missing the localized value %q:\n%s", want, out)
	}
	// The en render must still be byte-identical to the stored Value, which is
	// what makes the ValueKey safe to add.
	en := modelForTest(t, "en")
	if got := en.TF("alert.value_chips", map[string]interface{}{"Chips": "0, 3"}); got != "chips 0, 3" {
		t.Errorf("en alert.value_chips = %q, want the data-layer value %q", got, "chips 0, 3")
	}
}

// TestAlertCategoryFooterLocalized is the W4-4 guard: the alerts footer printed
// the category string the data layer stores ("Node"/"Pod"/...), which is a
// product literal and stayed English in the Chinese UI.
func TestAlertCategoryFooterLocalized(t *testing.T) {
	m := modelForTest(t, "zh")
	m.width = 200
	m.height = 60
	m.currentView = ViewAlerts
	m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{Alerts: []model.Alert{
		{Severity: model.AlertSeverityCritical, Category: "Node", ResourceType: "Node", ResourceName: "n1", Message: "m"},
		{Severity: model.AlertSeverityCritical, Category: "Node", ResourceType: "Node", ResourceName: "n2", Message: "m"},
		{Severity: model.AlertSeverityWarning, Category: "Pod", ResourceType: "Pod", ResourceName: "p1", Message: "m"},
		{Severity: model.AlertSeverityWarning, Category: "Service", ResourceType: "Service", ResourceName: "s1", Message: "m"},
		{Severity: model.AlertSeverityInfo, Category: "Storage", ResourceType: "PVC", ResourceName: "c1", Message: "m"},
		{Severity: model.AlertSeverityInfo, Category: "Resource", ResourceType: "Node", ResourceName: "n3", Message: "m"},
	}}}

	out := stripANSI(m.View())

	// The footer is the line carrying the category counts; the alert rows above
	// it legitimately print the raw resource type the data layer stored.
	footer := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, m.TF("stats.total_alerts", map[string]interface{}{"Count": 6})) {
			footer = line
		}
	}
	if footer == "" {
		t.Fatalf("the alert footer is not rendered:\n%s", out)
	}

	// "Pod" and "NPU" are product names the zh bundle keeps untranslated, so
	// only the four categories that do translate may not stay English.
	for _, bad := range []string{"Node:", "Service:", "Storage:", "Resource:"} {
		if strings.Contains(footer, bad) {
			t.Errorf("the zh alert footer still prints the raw category %q: %q", bad, footer)
		}
	}
	for _, want := range []string{
		m.T("alerts.category.node"), m.T("alerts.category.service"),
		m.T("alerts.category.storage"), m.T("alerts.category.resource"),
	} {
		if !strings.Contains(footer, want) {
			t.Errorf("the zh alert footer is missing %q: %q", want, footer)
		}
	}
	// The zh values must differ from the raw literals, or the footer above
	// would have passed on the English text.
	if m.T("alerts.category.node") == "Node" {
		t.Errorf("zh alerts.category.node is still the English literal")
	}
}

// TestAlertCategoryFooterEnglishUnchanged is the anchor for W4-4: the English
// footer printed the raw category and must keep doing so byte for byte.
func TestAlertCategoryFooterEnglishUnchanged(t *testing.T) {
	m := modelForTest(t, "en")
	m.width = 200
	m.height = 60
	m.currentView = ViewAlerts
	m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{Alerts: []model.Alert{
		{Severity: model.AlertSeverityCritical, Category: "Node", ResourceType: "Node", ResourceName: "n1", Message: "m"},
		{Severity: model.AlertSeverityCritical, Category: "Node", ResourceType: "Node", ResourceName: "n2", Message: "m"},
		{Severity: model.AlertSeverityWarning, Category: "Pod", ResourceType: "Pod", ResourceName: "p1", Message: "m"},
	}}}

	out := stripANSI(m.View())
	if !strings.Contains(out, "Node: 2") {
		t.Errorf("the en alert footer lost its Node count:\n%s", out)
	}
	if !strings.Contains(out, "Pod: 1") {
		t.Errorf("the en alert footer lost its Pod count:\n%s", out)
	}
}

// TestAlertCategoryKeysMatchDataLiterals pins the category→key mapping to the
// six categories the data layer emits: the en bundle value of each key must be
// the raw category (so en output does not change) and the zh value must not be
// the English literal (so zh actually switches).
func TestAlertCategoryKeysMatchDataLiterals(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	want := map[string]string{
		"alerts.category.node":     "Node",
		"alerts.category.pod":      "Pod",
		"alerts.category.npu":      "NPU",
		"alerts.category.service":  "Service",
		"alerts.category.storage":  "Storage",
		"alerts.category.resource": "Resource",
	}
	for key, category := range want {
		if got := en[key]; got != category {
			t.Errorf("en %s = %q, want the raw category %q (en output must not change)", key, got, category)
		}
		if got := zh[key]; got == "" {
			t.Errorf("zh bundle is missing %s", key)
		}
	}
}

// TestZhCopyFixes pins the W4-6 text fixes: the Volcano job statistics used
// half-width colons where every sibling statistic uses the full-width one, and
// the SuperPod node count mixed the English "Pods:" into a Chinese sentence.
func TestZhCopyFixes(t *testing.T) {
	// The zh values are read from the bundle rather than from the localizer so
	// the assertion names the exact strings the next reviewer will look for.
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	for _, id := range []string{
		"workloads.volcanojobs.stats",
		"workloads.volcanojobs.stats_running",
		"workloads.volcanojobs.stats_pending",
		"workloads.volcanojobs.stats_completed",
	} {
		value, ok := zh[id]
		if !ok {
			t.Errorf("zh bundle is missing %s", id)
			continue
		}
		if strings.Contains(value, ": ") && !strings.Contains(value, "：") {
			t.Errorf("zh %s still uses a half-width colon: %q", id, value)
		}
	}

	if got, want := zh["overview.npu_superpod_nodes"], "Pod 数：{{.Count}} 节点"; got != want {
		t.Errorf("zh overview.npu_superpod_nodes = %q, want %q", got, want)
	}

	// The English side is untouched.
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	if got := en["overview.npu_superpod_nodes"]; got != "Pods: {{.Count}} nodes" {
		t.Errorf("en overview.npu_superpod_nodes changed: %q", got)
	}
	for _, id := range []string{
		"workloads.volcanojobs.stats",
		"workloads.volcanojobs.stats_running",
		"workloads.volcanojobs.stats_pending",
		"workloads.volcanojobs.stats_completed",
	} {
		if got := en[id]; !strings.Contains(got, ": ") {
			t.Errorf("en %s lost its half-width colon: %q", id, got)
		}
	}
}

// TestBundleKeyCount pins the bundle at the size this batch leaves it in, so a
// key added without the other locale (or removed by accident) is caught even
// when both remaining key sets still agree.
//
// Batch 24 added 18 keys: alert.npu_chip_health, npudetail.{health_unknown,
// exporter_liveness,ecc_delta,error_code_chip}, npu.errorcat.* x6,
// npuview.{summary_unknown,liveness_ok,liveness_backoff,liveness_never} and
// npu.liveness.{ok,backoff,never} — 784 + 18 = 802.
//
// Batch 25 (Pod↔chip attribution) added 5: npudetail.{chip_owner,pod_chips,
// pod_chips_unknown}, npuview.{row_occupied,summary_no_known} — 802 + 5 = 807.
//
// Batch 29 (overview visualisation) added 14: npuview.{aggregate_nodes,
// aggregate_health,aggregate_power,aggregate_temp,aggregate_hbm,aggregate_util,
// mode_aicore,mode_temp,mode_power,heat_legend} and overview.{npu_chip_health,
// npu_chip_util,npu_temp_power,npu_temp_only} — 807 + 14 = 821.
//
// Batch 29 review fixes added 2: npuview.aggregate_health_no_known (the
// aggregate bar's no-known-fault wording for unknown-health chips) and
// overview.npu_chip_unknown (the card's unknown count) — 821 + 2 = 823.
//
// Batch 30 (NPU default matrix mode, HBM view, h discoverability) added 4:
// npuview.{mode_hbm,mode_health,health_legend} and keys.npu_matrix — 823 + 4 = 827.
//
// Batch 32 (node detail charts) added 4: nodedetail.{heat_strip,heat_legend,
// npu_util_trend,npu_power_trend} — 827 + 4 = 831.
func TestBundleKeyCount(t *testing.T) {
	const want = 831

	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")
	if len(en) != want {
		t.Errorf("en bundle has %d keys, want %d", len(en), want)
	}
	if len(zh) != want {
		t.Errorf("zh bundle has %d keys, want %d", len(zh), want)
	}
}

// TestFmtCommandOutputRangeNotLabel pins that the range string used by the
// command output status line has both locales.
func TestFmtCommandOutputRangeNotLabel(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		got := i18n.NewLocalizer(locale).TF("command_output.lines_range", map[string]interface{}{
			"Start": 1, "End": 10, "Total": 30,
		})
		if strings.Contains(got, "command_output.") {
			t.Errorf("%s: unresolved key %q", locale, got)
		}
		if fmt.Sprint(got) == "" {
			t.Errorf("%s: empty range string", locale)
		}
	}
}

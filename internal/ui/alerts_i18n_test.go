package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// Tests for alert message i18n in the alerts view: alerts carrying a
// MessageKey render through the localizer (English in en, Chinese in zh),
// while alerts without a key (raw cluster-provided text) fall back to the
// stored Message in every locale.
func TestRenderAlertRowMessageLocale(t *testing.T) {
	keyed := model.Alert{
		Severity:     model.AlertSeverityCritical,
		Category:     "Pod",
		AlertType:    model.AlertTypePodOOMKilled,
		ResourceType: "Pod",
		ResourceName: "app",
		Namespace:    "default",
		Message:      "Container worker was OOMKilled",
		MessageKey:   "alert.pod_oom_killed",
		MessageArgs:  map[string]interface{}{"Container": "worker"},
		Timestamp:    time.Now(),
	}
	raw := model.Alert{
		Severity:     model.AlertSeverityWarning,
		Category:     "NPU",
		AlertType:    model.AlertTypeNPUCheckFailed,
		ResourceType: "Node",
		ResourceName: "npu-node",
		Message:      "NPU inspection check NT_NPU_ECC_COUNT failed: ecc count exceed",
		Timestamp:    time.Now(),
	}

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	// Keyed alert: en renders the original English text, zh the translation.
	enKeyed := en.renderAlertRow(keyed)
	if !strings.Contains(enKeyed, "Container worker was OOMKilled") {
		t.Errorf("en keyed alert row %q does not contain the English message", enKeyed)
	}
	zhKeyed := zh.renderAlertRow(keyed)
	if !strings.Contains(zhKeyed, "容器 worker 已被 OOMKilled 终止") {
		t.Errorf("zh keyed alert row %q does not contain the Chinese message", zhKeyed)
	}
	if strings.Contains(zhKeyed, "Container worker was OOMKilled") {
		t.Errorf("zh keyed alert row %q still contains the English message", zhKeyed)
	}

	// Raw alert: no key, so both locales render the stored Message.
	for name, m := range map[string]*Model{"en": en, "zh": zh} {
		row := m.renderAlertRow(raw)
		if !strings.Contains(row, "NPU inspection check NT_NPU_ECC_COUNT failed: ecc count exceed") {
			t.Errorf("%s raw alert row %q does not contain the stored Message", name, row)
		}
	}
}

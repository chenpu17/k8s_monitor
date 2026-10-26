package datasource

import (
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// TestCollectAlertsSwitchNotHandleSeverity pins the severity of the switch
// fault alert per fault level. The management plane's NotHandle tier means it
// saw the event and decided no action is needed, and on real clusters that is
// the idle value — both nodes of the A3 pair carry a permanent NotHandle
// annotation, which used to pin two never-clearing warning alerts to the
// alerts view. NotHandle therefore reports as info; every other level keeps
// the warning (the switch path has no critical tier: the message carries the
// raw level for whoever knows the topology).
func TestCollectAlertsSwitchNotHandleSeverity(t *testing.T) {
	agg := &AggregatedDataSource{}

	cases := []struct {
		level string
		want  model.AlertSeverity
	}{
		{"NotHandle", model.AlertSeverityInfo},
		{"nothandle", model.AlertSeverityInfo}, // fault-info text casing is not guaranteed
		{"Warning", model.AlertSeverityWarning},
		{"UrgentNPU", model.AlertSeverityWarning},
	}
	for _, tc := range cases {
		nodes := npuTestNodes(&model.NodeData{
			Name:     "switch-node",
			NPUFault: &model.NPUFaultInfo{SwitchFaultLevel: tc.level, SwitchFaultName: "L1-1520"},
		})
		alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUSwitchFault)
		if len(alerts) != 1 {
			t.Fatalf("level %q: got %d switch alerts, want 1: %+v", tc.level, len(alerts), alerts)
		}
		if alerts[0].Severity != tc.want {
			t.Errorf("level %q: severity = %v, want %v", tc.level, alerts[0].Severity, tc.want)
		}
	}
}

// TestCollectAlertsSwitchNormalStaysSilent guards the firing condition while
// the severity logic changes: an empty level and a plain "Normal" must not
// produce an alert at all — downgrading NotHandle must not widen what fires.
func TestCollectAlertsSwitchNormalStaysSilent(t *testing.T) {
	agg := &AggregatedDataSource{}

	for _, level := range []string{"", "Normal"} {
		nodes := npuTestNodes(&model.NodeData{
			Name:     "switch-node",
			NPUFault: &model.NPUFaultInfo{SwitchFaultLevel: level},
		})
		if alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUSwitchFault); len(alerts) != 0 {
			t.Errorf("level %q: got %d switch alerts, want none", level, len(alerts))
		}
	}
}

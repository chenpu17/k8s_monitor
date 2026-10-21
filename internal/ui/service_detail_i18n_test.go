package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderServiceEndpoints bilingual lockdown: the English output must stay
// byte-identical to the previously hardcoded strings (including the leading
// indentation of the endpoint lines), and the Chinese output must switch.
// The colour profile is forced to Ascii by forcedAsciiProfile (defined in
// compact_alerts_test.go) so the goldens are plain text.
func TestRenderServiceEndpointsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	empty := en.renderServiceEndpoints(&model.ServiceData{Name: "svc-empty", Namespace: "default"})
	wantEmpty := "Endpoints\n\n  ⚠  No ready endpoints (0)\n  This service has no pods backing it or pods are not ready"
	if empty != wantEmpty {
		t.Errorf("en zero-endpoint render:\n got %q\nwant %q", empty, wantEmpty)
	}

	ready := en.renderServiceEndpoints(&model.ServiceData{Name: "svc-ready", Namespace: "default", EndpointCount: 3})
	wantReady := "Endpoints\n\n  ✓ 3 ready endpoint(s)"
	if ready != wantReady {
		t.Errorf("en ready-endpoint render:\n got %q\nwant %q", ready, wantReady)
	}
}

func TestRenderServiceEndpointsZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	empty := zh.renderServiceEndpoints(&model.ServiceData{Name: "svc-empty", Namespace: "default"})
	for _, noun := range []string{"端点", "无就绪端点 (0)", "该服务没有后端 Pod"} {
		if !strings.Contains(empty, noun) {
			t.Errorf("zh zero-endpoint render %q does not contain %q", empty, noun)
		}
	}
	for _, bad := range []string{"service.", "{{", "}}", "<no value>", "%!", "Endpoints", "No ready endpoints", "This service has no pods"} {
		if strings.Contains(empty, bad) {
			t.Errorf("zh zero-endpoint render %q contains %q", empty, bad)
		}
	}

	ready := zh.renderServiceEndpoints(&model.ServiceData{Name: "svc-ready", Namespace: "default", EndpointCount: 3})
	if !strings.Contains(ready, "✓ 3 个就绪端点") {
		t.Errorf("zh ready-endpoint render %q does not contain 个就绪端点", ready)
	}
	for _, bad := range []string{"{{", "}}", "<no value>", "%!", "ready endpoint(s)"} {
		if strings.Contains(ready, bad) {
			t.Errorf("zh ready-endpoint render %q contains %q", ready, bad)
		}
	}
}

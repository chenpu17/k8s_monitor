package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderEventDetail and its section renderers bilingual lockdown: the English
// output must stay byte-identical to the previously hardcoded strings, and the
// Chinese output must switch. The event Type (Normal/Warning/...) is a k8s API
// value rendered from the data, so it stays untranslated in both locales — the
// tests pin that behavior. Reason/Message render through the shared
// detail.field.* keys (see detail_field_shared_keys_test.go); Namespace
// reuses detail.namespace and the scroll indicator reuses the long
// detail.scroll_indicator variant, both byte-identical to the previously
// hardcoded strings.

func eventDetailFixture() *model.EventData {
	return &model.EventData{
		Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container",
		Count:             7,
		FirstTimestamp:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		LastTimestamp:     time.Date(2026, 1, 2, 3, 10, 6, 0, time.UTC),
		InvolvedObject:    "Pod/pod-a",
		InvolvedNamespace: "ns1",
	}
}

func TestRenderEventDetailHeaderEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	want := " ⚡ Event: BackOff   Warning"
	if got := en.renderEventDetailHeader(eventDetailFixture()); got != want {
		t.Errorf("en warning header:\n got %q\nwant %q", got, want)
	}

	normal := eventDetailFixture()
	normal.Type = "Normal"
	normal.Reason = "Scheduled"
	want = " ⚡ Event: Scheduled   Normal"
	if got := en.renderEventDetailHeader(normal); got != want {
		t.Errorf("en normal header:\n got %q\nwant %q", got, want)
	}

	// A type outside the two styled branches still renders the raw API value.
	other := eventDetailFixture()
	other.Type = "Error"
	other.Reason = "Failed"
	want = " ⚡ Event: Failed   Error"
	if got := en.renderEventDetailHeader(other); got != want {
		t.Errorf("en other-type header:\n got %q\nwant %q", got, want)
	}
}

func TestRenderEventDetailHeaderZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	got := zh.renderEventDetailHeader(eventDetailFixture())
	// The kind noun translates; the event Type stays the raw API value.
	if !strings.Contains(got, "⚡ 事件：BackOff") || !strings.Contains(got, "Warning") {
		t.Errorf("zh warning header %q must keep the API type and switch the noun", got)
	}
	if strings.Contains(got, "Event: BackOff") {
		t.Errorf("zh warning header %q still contains the English title", got)
	}
}

func TestRenderEventBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	want := " 📋 Basic Information \n\n" +
		"  Type: Warning\n" +
		"  Reason: BackOff\n" +
		"  Involved Object: Pod/pod-a\n" +
		"  Namespace: ns1\n" +
		"  Count: 7\n\n" +
		"  Message:\n" +
		"    Back-off restarting failed container"
	if got := en.renderEventBasicInfo(eventDetailFixture()); got != want {
		t.Errorf("en basic info with namespace:\n got %q\nwant %q", got, want)
	}

	noNs := eventDetailFixture()
	noNs.Type = "Normal"
	noNs.Reason = "Scheduled"
	noNs.Count = 1
	noNs.InvolvedNamespace = ""
	noNs.Message = "Successfully assigned ns1/pod-a to node-1"
	want = " 📋 Basic Information \n\n" +
		"  Type: Normal\n" +
		"  Reason: Scheduled\n" +
		"  Involved Object: Pod/pod-a\n" +
		"  Count: 1\n\n" +
		"  Message:\n" +
		"    Successfully assigned ns1/pod-a to node-1"
	if got := en.renderEventBasicInfo(noNs); got != want {
		t.Errorf("en basic info without namespace:\n got %q\nwant %q", got, want)
	}
}

func TestRenderEventBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	got := zh.renderEventBasicInfo(eventDetailFixture())
	for _, noun := range []string{
		"📋 基本信息", "类型: Warning", "原因: BackOff", "涉及对象: Pod/pod-a",
		"命名空间: ns1", "次数: 7", "消息:",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh basic info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Basic Information", "Type:", "Reason:", "Involved Object", "Count:",
		"Message:", "Namespace:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

func TestRenderEventTimingInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	want := " 🕐 Timing Information \n\n" +
		"  First Seen: 2026-01-02 03:04:05\n" +
		"  Last Seen: 2026-01-02 03:10:06\n" +
		"  Last Occurrence: " + eventDetailFixture().LastTimestamp.Local().Format("2006-01-02 15:04:05")
	if got := en.renderEventTimingInfo(eventDetailFixture()); got != want {
		t.Errorf("en timing info:\n got %q\nwant %q", got, want)
	}
}

func TestRenderEventTimingInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	got := zh.renderEventTimingInfo(eventDetailFixture())
	for _, noun := range []string{"🕐 时间信息", "首次出现: 2026-01-02 03:04:05", "最近出现: 2026-01-02 03:10:06", "最近发生:"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh timing info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Timing Information", "First Seen", "Last Seen", "Last Occurrence",
		"{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh timing info %q contains %q", got, bad)
		}
	}
}

func TestRenderEventDetailNoSelectedAndScroll(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.width = 120
	if got := en.renderEventDetail(); got != "No event selected" {
		t.Errorf("en no-selection render:\n got %q\nwant %q", got, "No event selected")
	}

	en.selectedEvent = eventDetailFixture()
	en.height = 1000
	if got := en.renderEventDetail(); strings.Contains(got, "[Lines") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", got)
	}

	// The event view renders the long variant of detail.scroll_indicator
	// (byte-identical to the previously hardcoded line).
	en.height = 12 // maxVisible = 4
	got := en.renderEventDetail()
	if !strings.Contains(got, "[Lines 1-4 of ") ||
		!strings.Contains(got, "] (↑/↓ to scroll, PgUp/PgDn for page)") {
		t.Errorf("short render must show the long scroll indicator:\n%s", got)
	}

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	zh.width = 120
	if got := zh.renderEventDetail(); got != "未选择事件" {
		t.Errorf("zh no-selection render:\n got %q\nwant %q", got, "未选择事件")
	}
	zh.selectedEvent = eventDetailFixture()
	zh.height = 12
	got = zh.renderEventDetail()
	if !strings.Contains(got, "[第 1-4 行") || !strings.Contains(got, "（↑/↓ 滚动，PgUp/PgDn 翻页）") {
		t.Errorf("zh short render must show the long scroll indicator:\n%s", got)
	}
}

package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderPVCDetail bilingual lockdown: the English output must stay
// byte-identical to the previously hardcoded strings (including leading
// indentation), and the Chinese output must switch. The Labels/Annotations/
// "... and N more"/"(X ago)" strings are shared with the PV detail view via
// the detail.storage.* keys (see storage_detail_shared_keys_test.go), and the
// scroll indicator is the third (viewing) variant detail.scroll_indicator_viewing.
// The PVC Phase values (Bound/Pending/Lost) are k8s API values and stay
// untranslated. The Created line's duration has second-level jitter
// (formatDuration), so the goldens pin the line up to "Created: " and pin the
// " ago)" tail instead of an exact duration.

const pvcGoldenWidth = 120

func pvcGoldenSeparator() string {
	// renderSeparator renders width-2 dashes.
	return strings.Repeat("─", pvcGoldenWidth-2)
}

func pvcFullFixture() *model.PVCData {
	return &model.PVCData{
		Name: "pvc-b", Namespace: "ns1", Status: "Bound", Volume: "pv-1",
		Capacity:         5 * 1024 * 1024 * 1024,
		RequestedStorage: 10 * 1024 * 1024 * 1024,
		StorageClass:     "fast", VolumeMode: "Filesystem",
		AccessModes: []string{"RWO", "RWX"},
		Labels:      map[string]string{"app": "web", "tier": "frontend"},
		Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5", "k6": "v6", "k7": "v7"},
		UsedBytes:   3 * 1024 * 1024 * 1024,
	}
}

func TestRenderPVCDetailEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.width = pvcGoldenWidth
	en.height = 1000
	sep := pvcGoldenSeparator()

	// Minimal bound claim: unset volume/storageclass render their placeholders.
	en.selectedPVC = &model.PVCData{Name: "pvc-a", Namespace: "ns1", Status: "Bound"}
	want := " 📋 PersistentVolumeClaim: pvc-a \n\n" +
		"📋 Basic Information\n" + sep + "\n" +
		"  Namespace: ns1\n" +
		"  Status: Bound\n" +
		"  Volume: <not bound>\n\n" +
		"📝 Spec\n" + sep + "\n" +
		"  Requested: 0\n" +
		"  Capacity: <unknown>\n\n" +
		"⚙️  Storage Configuration\n" + sep + "\n" +
		"  StorageClass: <none>\n\n" +
		"📅 Lifecycle\n" + sep + "\n" +
		"  Created: "
	got := en.renderPVCDetail()
	if !strings.HasPrefix(got, want) || !strings.HasSuffix(got, " ago)") {
		t.Errorf("en minimal bound render drifted:\n got %q\nwant prefix %q and suffix %q", got, want, " ago)")
	}

	// Fully populated claim with the Used line, labels, and truncated
	// annotations ("... and 2 more").
	full := pvcFullFixture()
	en.selectedPVC = full
	want = " 📋 PersistentVolumeClaim: pvc-b \n\n" +
		"📋 Basic Information\n" + sep + "\n" +
		"  Namespace: ns1\n" +
		"  Status: Bound\n" +
		"  Volume: pv-1\n\n" +
		"📝 Spec\n" + sep + "\n" +
		"  Requested: 10.0Gi\n" +
		"  Capacity: 5.0Gi\n" +
		"  Used: 3.0Gi (60.0%)\n\n" +
		"⚙️  Storage Configuration\n" + sep + "\n" +
		"  StorageClass: fast\n" +
		"  Volume Mode: Filesystem\n" +
		"  Access Modes: RWO, RWX\n\n" +
		"🏷️  Labels\n" + sep + "\n" +
		"  app: web\n" +
		"  tier: frontend\n\n" +
		"📝 Annotations\n" + sep + "\n" +
		"  k1: v1\n" +
		"  k2: v2\n" +
		"  k3: v3\n" +
		"  k4: v4\n" +
		"  k5: v5\n" +
		"  ... and 2 more\n\n" +
		"📅 Lifecycle\n" + sep + "\n" +
		"  Created: "
	got = en.renderPVCDetail()
	if !strings.HasPrefix(got, want) || !strings.HasSuffix(got, " ago)") {
		t.Errorf("en full render drifted:\n got %q\nwant prefix %q and suffix %q", got, want, " ago)")
	}

	// Oversized label values truncate with the ASCII ellipsis.
	long := &model.PVCData{
		Name: "pvc-l", Namespace: "ns1", Status: "Bound",
		Labels: map[string]string{"verylonglabelkeyname": strings.Repeat("v", 65)},
	}
	en.selectedPVC = long
	want = "  verylonglabelkeyname: " + strings.Repeat("v", 60) + "..."
	if !strings.Contains(en.renderPVCDetail(), want) {
		t.Errorf("en long label truncation drifted, want %q", want)
	}

	// Ten annotations hide the section entirely (rendered only below 10).
	ten := &model.PVCData{
		Name: "pvc-t", Namespace: "ns1", Status: "Bound",
		Annotations: map[string]string{"a1": "1", "a2": "2", "a3": "3", "a4": "4", "a5": "5", "a6": "6", "a7": "7", "a8": "8", "a9": "9", "a10": "10"},
	}
	en.selectedPVC = ten
	if got := en.renderPVCDetail(); strings.Contains(got, "📝 Annotations") {
		t.Errorf("en render with 10 annotations must hide the section:\n%s", got)
	}

	// Pending/Lost/other phases render the raw API values.
	for _, phase := range []string{"Pending", "Lost", "Weird"} {
		p := &model.PVCData{Name: "pvc-a", Namespace: "ns1", Status: phase}
		en.selectedPVC = p
		if got := en.renderPVCDetail(); !strings.Contains(got, "  Status: "+phase) {
			t.Errorf("en status %q render drifted:\n%s", phase, got)
		}
	}
}

func TestRenderPVCDetailZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	zh.width = pvcGoldenWidth
	zh.height = 1000
	zh.selectedPVC = pvcFullFixture()

	got := zh.renderPVCDetail()
	for _, noun := range []string{
		"📋 持久卷声明: pvc-b", "命名空间: ns1", "已用: 3.0Gi (60.0%)",
		"⚙️  存储配置", "存储类: fast", "卷模式: Filesystem", "访问模式: RWO, RWX",
		"🏷️  标签", "📝 注解", "... 还有 2 条", "创建时间: ", "前）",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh pvc detail %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Basic Information", "Storage Configuration", "StorageClass:", "Labels",
		"Annotations", "... and ", "Used:", "Created:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh pvc detail %q contains %q", got, bad)
		}
	}
}

func TestRenderPVCDetailScrollEn(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.width = pvcGoldenWidth
	en.selectedPVC = pvcFullFixture()

	en.height = 1000
	if got := en.renderPVCDetail(); strings.Contains(got, "(viewing ") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", got)
	}

	// The third scroll variant: a "(viewing ...)" line, byte-distinct from
	// both detail.scroll_indicator variants.
	en.height = 12 // maxVisible = 5
	got := en.renderPVCDetail()
	if !strings.Contains(got, "(viewing 1-5 of ") ||
		!strings.Contains(got, " lines, use ↑↓ or PgUp/PgDn to scroll)") {
		t.Errorf("short render must show the viewing scroll indicator:\n%s", got)
	}
	if strings.Contains(got, "PgUp/PgDn for page") {
		t.Errorf("pvc must not render the long scroll variant:\n%s", got)
	}

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	zh.width = pvcGoldenWidth
	zh.selectedPVC = pvcFullFixture()
	zh.height = 12
	got = zh.renderPVCDetail()
	if !strings.Contains(got, "（查看第 1-5 行") ||
		!strings.Contains(got, "↑↓ 或 PgUp/PgDn 滚动）") {
		t.Errorf("zh short render must show the viewing scroll indicator:\n%s", got)
	}
	if strings.Contains(got, "翻页）") {
		t.Errorf("zh pvc must not render the long scroll variant:\n%s", got)
	}
}

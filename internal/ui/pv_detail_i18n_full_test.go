package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderPVDetail bilingual lockdown: the English output must stay
// byte-identical to the previously hardcoded strings, and the Chinese output
// must switch. Labels/Annotations/"... and N more"/"(X ago)" are shared with
// the PVC detail view via detail.storage.* (see
// storage_detail_shared_keys_test.go); the unset-storageclass placeholder
// "<none>" renders through detail.pv.none (sibling of detail.pvc.none, both
// zh "<无>"). The PV Phase values (Bound/Available/Released/Failed) are k8s
// API values and stay untranslated. As in the PVC goldens, the Created line
// is pinned up to "Created: " plus the " ago)" tail (second-level jitter in
// formatDuration).

const pvGoldenWidth = 120

func pvGoldenSeparator() string {
	// renderSeparator renders width-2 dashes.
	return strings.Repeat("─", pvGoldenWidth-2)
}

func pvFullFixture() *model.PVData {
	return &model.PVData{
		Name: "pv-b", Status: "Bound", Capacity: 10 * 1024 * 1024 * 1024,
		StorageClass: "fast", ReclaimPolicy: "Delete", VolumeMode: "Filesystem",
		VolumeType: "NFS", AccessModes: []string{"RWX"},
		Claim:       "ns1/pvc-b",
		Labels:      map[string]string{"app": "web", "tier": "frontend"},
		Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5", "k6": "v6", "k7": "v7"},
	}
}

func TestRenderPVDetailEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.width = pvGoldenWidth
	en.height = 1000
	sep := pvGoldenSeparator()

	// Minimal bound volume: unset storageclass renders the <none> placeholder.
	en.selectedPV = &model.PVData{Name: "pv-a", Status: "Bound"}
	want := " 📦 PersistentVolume: pv-a \n\n" +
		"📋 Basic Information\n" + sep + "\n" +
		"  Status: Bound\n" +
		"  Capacity: 0\n" +
		"  StorageClass: <none>\n" +
		"  Reclaim Policy: \n\n" +
		"📦 Claim Information\n" + sep + "\n" +
		"  Bound to: <not bound>\n\n" +
		"📅 Lifecycle\n" + sep + "\n" +
		"  Created: "
	got := en.renderPVDetail()
	if !strings.HasPrefix(got, want) || !strings.HasSuffix(got, " ago)") {
		t.Errorf("en minimal render drifted:\n got %q\nwant prefix %q and suffix %q", got, want, " ago)")
	}

	// Fully populated volume with labels and truncated annotations.
	full := pvFullFixture()
	en.selectedPV = full
	want = " 📦 PersistentVolume: pv-b \n\n" +
		"📋 Basic Information\n" + sep + "\n" +
		"  Status: Bound\n" +
		"  Capacity: 10.0Gi\n" +
		"  StorageClass: fast\n" +
		"  Reclaim Policy: Delete\n" +
		"  Volume Mode: Filesystem\n" +
		"  Type: NFS\n" +
		"  Access Modes: RWX\n\n" +
		"📦 Claim Information\n" + sep + "\n" +
		"  Bound to: ns1/pvc-b\n\n" +
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
	got = en.renderPVDetail()
	if !strings.HasPrefix(got, want) || !strings.HasSuffix(got, " ago)") {
		t.Errorf("en full render drifted:\n got %q\nwant prefix %q and suffix %q", got, want, " ago)")
	}

	// Oversized label values truncate with the ASCII ellipsis.
	long := &model.PVData{
		Name: "pv-l", Status: "Available",
		Labels: map[string]string{"verylonglabelkeyname": strings.Repeat("v", 65)},
	}
	en.selectedPV = long
	want = "  verylonglabelkeyname: " + strings.Repeat("v", 60) + "..."
	if !strings.Contains(en.renderPVDetail(), want) {
		t.Errorf("en long label truncation drifted, want %q", want)
	}

	// Phases render the raw API values.
	for _, phase := range []string{"Bound", "Available", "Released", "Failed"} {
		p := &model.PVData{Name: "pv-a", Status: phase}
		en.selectedPV = p
		if got := en.renderPVDetail(); !strings.Contains(got, "  Status: "+phase) {
			t.Errorf("en status %q render drifted:\n%s", phase, got)
		}
	}
}

func TestRenderPVDetailZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	zh.width = pvGoldenWidth
	zh.height = 1000
	zh.selectedPV = pvFullFixture()

	got := zh.renderPVDetail()
	for _, noun := range []string{
		"📦 持久卷: pv-b", "容量: 10.0Gi", "存储类: fast", "回收策略: Delete",
		"卷模式: Filesystem", "类型: NFS", "访问模式: RWX",
		"📦 声明信息", "绑定到: ns1/pvc-b",
		"🏷️  标签", "📝 注解", "... 还有 2 条", "创建时间: ", "前）",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh pv detail %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Basic Information", "Claim Information", "Bound to:", "StorageClass:",
		"Reclaim Policy", "Labels", "Annotations", "... and ", "Created:",
		"<none>", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh pv detail %q contains %q", got, bad)
		}
	}

	// Unset storageclass renders the translated placeholder.
	empty := &model.PVData{Name: "pv-a", Status: "Bound"}
	zh.selectedPV = empty
	if got := zh.renderPVDetail(); !strings.Contains(got, "<无>") {
		t.Errorf("zh pv unset storageclass must render <无>:\n%s", got)
	}
}

func TestRenderPVDetailScrollEn(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.width = pvGoldenWidth
	en.selectedPV = pvFullFixture()

	en.height = 1000
	if got := en.renderPVDetail(); strings.Contains(got, "(viewing ") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", got)
	}

	// The third scroll variant, shared with the PVC view.
	en.height = 12 // maxVisible = 5
	got := en.renderPVDetail()
	if !strings.Contains(got, "(viewing 1-5 of ") ||
		!strings.Contains(got, " lines, use ↑↓ or PgUp/PgDn to scroll)") {
		t.Errorf("short render must show the viewing scroll indicator:\n%s", got)
	}
	if strings.Contains(got, "PgUp/PgDn for page") {
		t.Errorf("pv must not render the long scroll variant:\n%s", got)
	}
}

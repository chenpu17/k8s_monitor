package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The two storage detail views (PV / PVC) are template siblings: their
// isomorphic strings share one detail.storage.* bundle key per literal
// instead of one copy per family, and the scroll indicator is the third
// variant detail.scroll_indicator_viewing shared by both. This test pins the
// sharing both ways:
//   - each sibling must render the byte-identical shared composition (a
//     forked per-family copy with drifted wording turns exactly this test red
//     while every per-family golden stays green);
//   - the compositions are pinned by hardcoded literals, so editing a shared
//     key's value in the bundle turns this test AND the golden tests of both
//     family files red at once — the drift-protection property the shared
//     namespace exists for.

func storageSharedPVC() *model.PVCData {
	return &model.PVCData{
		Name: "pvc-b", Namespace: "ns1", Status: "Bound", Volume: "pv-1",
		Labels:      map[string]string{"app": "web", "tier": "frontend"},
		Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5", "k6": "v6", "k7": "v7"},
	}
}

func storageSharedPV() *model.PVData {
	return &model.PVData{
		Name: "pv-b", Status: "Bound",
		Labels:      map[string]string{"app": "web", "tier": "frontend"},
		Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5", "k6": "v6", "k7": "v7"},
	}
}

func TestStorageDetailSharedKeysLockedAcrossSiblings(t *testing.T) {
	forcedAsciiProfile(t)

	compositions := map[string]struct {
		labels      string
		annotations string
		viewing     string
	}{
		"en": {
			labels: "🏷️  Labels\n" + strings.Repeat("─", 118) + "\n  app: web\n  tier: frontend",
			annotations: "📝 Annotations\n" + strings.Repeat("─", 118) + "\n" +
				"  k1: v1\n  k2: v2\n  k3: v3\n  k4: v4\n  k5: v5\n  ... and 2 more",
			viewing: "(viewing 1-5 of ",
		},
		"zh": {
			labels: "🏷️  标签\n" + strings.Repeat("─", 118) + "\n  app: web\n  tier: frontend",
			annotations: "📝 注解\n" + strings.Repeat("─", 118) + "\n" +
				"  k1: v1\n  k2: v2\n  k3: v3\n  k4: v4\n  k5: v5\n  ... 还有 2 条",
			viewing: "（查看第 1-5 行",
		},
	}

	for locale, want := range compositions {
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
		m.width = 120
		m.height = 1000

		// Labels + annotations sections: byte-identical across both siblings.
		m.selectedPVC = storageSharedPVC()
		m.selectedPV = storageSharedPV()
		pvcOut := m.renderPVCDetail()
		pvOut := m.renderPVDetail()
		for section, comp := range map[string]string{"labels": want.labels, "annotations": want.annotations} {
			if !strings.Contains(pvcOut, comp) {
				t.Errorf("[%s] pvc %s section must render the shared composition:\n%s", locale, section, pvcOut)
			}
			if !strings.Contains(pvOut, comp) {
				t.Errorf("[%s] pv %s section must render the shared composition:\n%s", locale, section, pvOut)
			}
		}

		// The "(X ago)" created suffix: byte-identical rendering is pinned by
		// the per-family goldens; here both siblings must carry the shared
		// key's parenthesized form (zh full-width).
		if locale == "en" {
			for _, out := range map[string]string{"pvc": pvcOut, "pv": pvOut} {
				if !strings.Contains(out, " ago)") {
					t.Errorf("[%s] %s created line must end with the shared \"(DUR ago)\" form:\n%s", locale, out, out)
				}
			}
		} else {
			for _, out := range map[string]string{"pvc": pvcOut, "pv": pvOut} {
				if !strings.Contains(out, "前）") || !strings.Contains(out, "（") {
					t.Errorf("[%s] %s created line must use the shared full-width form:\n%s", locale, out, out)
				}
			}
		}

		// Scroll indicator: both siblings render the viewing variant, and
		// neither renders the long detail.scroll_indicator variant.
		m.height = 12 // maxVisible = 5
		m.selectedPVC = storageSharedPVC()
		m.selectedPV = storageSharedPV()
		pvcScroll := m.renderPVCDetail()
		pvScroll := m.renderPVDetail()
		for family, out := range map[string]string{"pvc": pvcScroll, "pv": pvScroll} {
			if !strings.Contains(out, want.viewing) {
				t.Errorf("[%s] %s scroll render must show the viewing variant:\n%s", locale, family, out)
			}
			if locale == "en" && strings.Contains(out, "PgUp/PgDn for page") {
				t.Errorf("[%s] %s must not render the long scroll variant:\n%s", locale, family, out)
			}
			if locale == "zh" && strings.Contains(out, "翻页）") {
				t.Errorf("[%s] %s must not render the long scroll variant:\n%s", locale, family, out)
			}
		}
	}
}

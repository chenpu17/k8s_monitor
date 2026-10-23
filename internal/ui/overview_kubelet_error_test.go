package ui

// The overview's kubelet-error display point: keyed denials render through the
// localizer (zh copy byte-identical to the raw text, en copy in English where
// the raw path used to leak Chinese), while unkeyed raw node errors keep the
// stored-text fallback byte for byte.

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The historical Message() output of the default denial, frozen as the zh
// anchor for the overview render.
const zhDefaultDenial = "当前凭证无法访问 kubelet proxy (需要 get nodes/proxy 权限) • 请执行 `kubectl auth can-i get nodes/proxy` 或授予相应 RBAC"

func kubeletErrorSummary(errText string) *model.ClusterSummary {
	return &model.ClusterSummary{
		TotalNodes:       2,
		NodesWithMetrics: 0,
		KubeletError:     errText,
	}
}

func TestOverviewKubeletErrorKeyedRender(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	t.Run("zh keyed render is byte-identical to raw fallback", func(t *testing.T) {
		keyed := zh.renderClusterLoadCompact(kubeletErrorSummaryWithKey(zhDefaultDenial, "diagnostic.proxy_denied_default", nil))
		raw := zh.renderClusterLoadCompact(kubeletErrorSummary(zhDefaultDenial))
		if keyed != raw {
			t.Errorf("zh keyed render differs from raw fallback:\nkeyed:\n%s\nraw:\n%s", keyed, raw)
		}
		if !strings.Contains(keyed, truncateText(zhDefaultDenial, 60)) {
			t.Errorf("zh render lost the denial text:\n%s", keyed)
		}
	})

	t.Run("en keyed render is English", func(t *testing.T) {
		out := en.renderClusterLoadCompact(kubeletErrorSummaryWithKey(zhDefaultDenial, "diagnostic.proxy_denied_default", nil))
		for _, bad := range []string{"当前凭证", "请执行"} {
			if strings.Contains(out, bad) {
				t.Errorf("en render leaked Chinese copy %q:\n%s", bad, out)
			}
		}
		// The 95-cell cluster-load box hard-wraps long lines, so assert the
		// keyed text per wrapped fragment: the English prefix on the first
		// line and the truncateText(60) cut marker on the next.
		if !strings.Contains(out, "Current identity cannot access the kubelet proxy") {
			t.Errorf("en render missing keyed English prefix:\n%s", out)
		}
		if !strings.Contains(out, "(require...") {
			t.Errorf("en render missing the 60-cell truncation marker of the keyed text:\n%s", out)
		}
	})

	t.Run("access review failure key renders with args", func(t *testing.T) {
		raw := "Access review failed: sar create forbidden" + " • 请执行 `kubectl auth can-i get nodes/proxy` 或授予相应 RBAC"
		args := map[string]interface{}{"Error": "sar create forbidden"}

		keyedZh := zh.renderClusterLoadCompact(kubeletErrorSummaryWithKey(raw, "diagnostic.access_review_failed", args))
		rawZh := zh.renderClusterLoadCompact(kubeletErrorSummary(raw))
		if keyedZh != rawZh {
			t.Errorf("zh keyed render differs from raw fallback:\nkeyed:\n%s\nraw:\n%s", keyedZh, rawZh)
		}

		keyedEn := en.renderClusterLoadCompact(kubeletErrorSummaryWithKey(raw, "diagnostic.access_review_failed", args))
		if !strings.Contains(keyedEn, "Access review failed: sar create forbidden") {
			t.Errorf("en render missing access review failure text:\n%s", keyedEn)
		}
		if strings.Contains(keyedEn, "请执行") {
			t.Errorf("en render leaked Chinese suffix:\n%s", keyedEn)
		}
	})

	t.Run("unkeyed raw error keeps the stored text", func(t *testing.T) {
		raw := "x509: certificate signed by unknown authority"
		out := en.renderClusterLoadCompact(kubeletErrorSummary(raw))
		if !strings.Contains(out, truncateText(raw, 60)) {
			t.Errorf("unkeyed raw error not rendered as stored:\n%s", out)
		}
	})
}

func kubeletErrorSummaryWithKey(errText, key string, args map[string]interface{}) *model.ClusterSummary {
	s := kubeletErrorSummary(errText)
	s.KubeletErrorKey = key
	s.KubeletErrorArgs = args
	return s
}

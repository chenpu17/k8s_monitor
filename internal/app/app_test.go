package app

import (
	"context"
	"strings"
	"testing"
)

// The UI layer (internal/ui/action_menu.go) discovers Describe/YAML support by
// type-asserting the DataProvider against anonymous interfaces at runtime:
//
//	apiClient, ok := m.dataProvider.(interface {
//	    DescribePod(ctx context.Context, namespace, podName string) (string, error)
//	    DescribeNode(ctx context.Context, nodeName string) (string, error)
//	})
//
// Nothing links those interfaces to *App at compile time, so a signature drift
// on either side makes the assertion fail silently: the action menu then just
// reports "API client does not support describe functionality" instead of
// showing the output, with no build error and no failing test. The two
// declarations below are the compile-time guard for that contract.
//
// They duplicate the interface literals verbatim on purpose. When a signature
// changes in action_menu.go, this file MUST be updated in the same commit —
// otherwise the guard keeps compiling while protecting nothing.
var _ interface {
	DescribePod(ctx context.Context, namespace, podName string) (string, error)
	DescribeNode(ctx context.Context, nodeName string) (string, error)
} = (*App)(nil)

var _ interface {
	GetPodYAML(ctx context.Context, namespace, podName string) (string, error)
	GetNodeYAML(ctx context.Context, nodeName string) (string, error)
} = (*App)(nil)

// TestActionMenuTypeAssertionsMatchApp reproduces the exact runtime assertion
// performed by action_menu.go so a failure points at the UI-side contract
// rather than at an unrelated compile error.
func TestActionMenuTypeAssertionsMatchApp(t *testing.T) {
	var provider interface{} = (*App)(nil)

	if _, ok := provider.(interface {
		DescribePod(ctx context.Context, namespace, podName string) (string, error)
		DescribeNode(ctx context.Context, nodeName string) (string, error)
	}); !ok {
		t.Error("action_menu.go Describe assertion no longer matches *App")
	}

	if _, ok := provider.(interface {
		GetPodYAML(ctx context.Context, namespace, podName string) (string, error)
		GetNodeYAML(ctx context.Context, nodeName string) (string, error)
	}); !ok {
		t.Error("action_menu.go GetYAML assertion no longer matches *App")
	}

	// The logs path in internal/ui/model.go uses the same pattern.
	if _, ok := provider.(interface {
		GetPodLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64) (string, error)
	}); !ok {
		t.Error("model.go GetPodLogs assertion no longer matches *App")
	}
}

// TestAppDescribeNilDataSource verifies the defensive branch taken before the
// data source is initialised (e.g. when App is constructed but Run() failed).
// It must return an error, not panic on a nil dereference.
func TestAppDescribeNilDataSource(t *testing.T) {
	a := &App{}
	ctx := context.Background()

	cases := []struct {
		name string
		call func() (string, error)
	}{
		{"DescribePod", func() (string, error) { return a.DescribePod(ctx, "ns", "pod") }},
		{"DescribeNode", func() (string, error) { return a.DescribeNode(ctx, "node") }},
		{"GetPodYAML", func() (string, error) { return a.GetPodYAML(ctx, "ns", "pod") }},
		{"GetNodeYAML", func() (string, error) { return a.GetNodeYAML(ctx, "node") }},
		{"GetPodLogs", func() (string, error) { return a.GetPodLogs(ctx, "ns", "pod", "c", 10) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.call()
			if err == nil {
				t.Fatalf("%s with nil data source: want error, got nil (output=%q)", tc.name, out)
			}
			if !strings.Contains(err.Error(), "not initialized") {
				t.Errorf("%s: error = %q, want it to mention the data source is not initialized", tc.name, err)
			}
		})
	}
}

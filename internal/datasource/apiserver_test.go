package datasource

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// writeTestKubeconfig writes a minimal kubeconfig pointing at an unreachable
// host. NewAPIServerClient never dials the API Server, so the file only has to
// parse.
func writeTestKubeconfig(t *testing.T) string {
	t.Helper()

	const kubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: test-token
`

	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("failed to write test kubeconfig: %v", err)
	}
	return path
}

// TestNewAPIServerClientAppliesRequestBudget pins the client-side request
// budget: without it client-go throttles a full refresh to QPS=5/Burst=10 and
// lets a hung API Server block the refresh loop indefinitely.
func TestNewAPIServerClientAppliesRequestBudget(t *testing.T) {
	kubeconfig := writeTestKubeconfig(t)

	cases := []struct {
		name        string
		timeout     time.Duration
		wantTimeout time.Duration
	}{
		{"configured timeout", 3 * time.Second, 3 * time.Second},
		{"zero falls back to default", 0, defaultAPIServerTimeout},
		{"negative falls back to default", -time.Second, defaultAPIServerTimeout},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewAPIServerClient(kubeconfig, "", tc.timeout, zap.NewNop())
			if err != nil {
				t.Fatalf("NewAPIServerClient returned error: %v", err)
			}

			config := client.GetConfig()
			if config.Timeout != tc.wantTimeout {
				t.Errorf("config.Timeout = %s, want %s", config.Timeout, tc.wantTimeout)
			}
			if config.QPS != apiServerQPS {
				t.Errorf("config.QPS = %v, want %v", config.QPS, float32(apiServerQPS))
			}
			if config.Burst != apiServerBurst {
				t.Errorf("config.Burst = %v, want %v", config.Burst, apiServerBurst)
			}
		})
	}
}

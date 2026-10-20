package datasource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// newEventListClient returns an APIServerClient backed by a real clientset
// whose API Server is an in-process HTTP server serving the given events. The
// client under test therefore exercises the same list/convert/sort/limit path
// it uses against a cluster.
func newEventListClient(t *testing.T, events []corev1.Event) *APIServerClient {
	t.Helper()

	body, err := json.Marshal(corev1.EventList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "EventList"},
		Items:    events,
	})
	if err != nil {
		t.Fatalf("failed to marshal event list: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	clientset, err := kubernetes.NewForConfig(&rest.Config{
		Host:          srv.URL,
		ContentConfig: rest.ContentConfig{ContentType: "application/json"},
	})
	if err != nil {
		t.Fatalf("failed to build clientset: %v", err)
	}

	return &APIServerClient{clientset: clientset, logger: zap.NewNop()}
}

// TestGetEvents pins the contract the aggregated data source relies on: every
// event type survives the list, the result is ordered most-recent-first, and
// the limit keeps the newest events rather than an arbitrary subset.
//
// corev1.Event.Type is a free-form string the API Server does not constrain to
// Normal/Warning, so "Error" is a legal value and must not be dropped on the
// way to the summary.
func TestGetEvents(t *testing.T) {
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	events := []corev1.Event{
		{
			ObjectMeta:    metav1.ObjectMeta{Name: "e-normal", Namespace: "default"},
			Type:          "Normal",
			Reason:        "Scheduled",
			LastTimestamp: metav1.NewTime(base.Add(3 * time.Minute)),
		},
		{
			ObjectMeta:    metav1.ObjectMeta{Name: "e-warning", Namespace: "default"},
			Type:          "Warning",
			Reason:        "BackOff",
			LastTimestamp: metav1.NewTime(base.Add(2 * time.Minute)),
		},
		{
			ObjectMeta:    metav1.ObjectMeta{Name: "e-error", Namespace: "default"},
			Type:          "Error",
			Reason:        "Custom",
			LastTimestamp: metav1.NewTime(base.Add(1 * time.Minute)),
		},
	}

	tests := []struct {
		name       string
		eventTypes []string
		limit      int
		want       []string // "Type/Reason", most recent first
	}{
		{
			name:  "no type filter keeps every type",
			limit: 0,
			want:  []string{"Normal/Scheduled", "Warning/BackOff", "Error/Custom"},
		},
		{
			name:       "explicit type filter is still honoured",
			eventTypes: []string{"Warning"},
			limit:      0,
			want:       []string{"Warning/BackOff"},
		},
		{
			name:       "type filter matching nothing yields no events",
			eventTypes: []string{"Info"},
			limit:      0,
			want:       []string{},
		},
		{
			name:  "limit keeps the most recent events",
			limit: 2,
			want:  []string{"Normal/Scheduled", "Warning/BackOff"},
		},
		{
			name:  "limit larger than the result set keeps everything",
			limit: 10,
			want:  []string{"Normal/Scheduled", "Warning/BackOff", "Error/Custom"},
		},
		{
			name:  "non-positive limit means unlimited",
			limit: -1,
			want:  []string{"Normal/Scheduled", "Warning/BackOff", "Error/Custom"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newEventListClient(t, events)

			got, err := client.GetEvents(context.Background(), "", tc.eventTypes, tc.limit)
			if err != nil {
				t.Fatalf("GetEvents returned error: %v", err)
			}

			gotKeys := make([]string, 0, len(got))
			for _, e := range got {
				gotKeys = append(gotKeys, e.Type+"/"+e.Reason)
			}

			if !reflect.DeepEqual(gotKeys, tc.want) {
				t.Errorf("GetEvents(eventTypes=%v, limit=%d) = %v, want %v",
					tc.eventTypes, tc.limit, gotKeys, tc.want)
			}
		})
	}
}

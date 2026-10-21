package datasource

import (
	"reflect"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Real annotation values read from the live Ascend910B1 node (8 chips).
const (
	realDeviceTopology = `{"topologys":[{"type":"HCCS","groups":["0,1,2,3,4,5,6,7"]}]}`
	realRankTable      = `{"device":[` +
		`{"device_ip":"29.11.129.127","device_id":"0","tor_port":"18:1","tor_ip":"29.79.161.163"},` +
		`{"device_ip":"29.11.182.239","device_id":"1","tor_port":"18:2","tor_ip":"29.79.161.163"},` +
		`{"device_ip":"29.11.159.1","device_id":"2","tor_port":"18:1","tor_ip":"29.79.161.169"},` +
		`{"device_ip":"29.11.108.246","device_id":"3","tor_port":"18:2","tor_ip":"29.79.161.169"},` +
		`{"device_ip":"29.11.73.127","device_id":"4","tor_port":"17:2","tor_ip":"29.79.161.169"},` +
		`{"device_ip":"29.11.61.222","device_id":"5","tor_port":"17:1","tor_ip":"29.79.161.169"},` +
		`{"device_ip":"29.11.45.20","device_id":"6","tor_port":"17:2","tor_ip":"29.79.161.163"},` +
		`{"device_ip":"29.11.173.142","device_id":"7","tor_port":"17:1","tor_ip":"29.79.161.163"}]}`
)

func TestParseNPUTopology(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        []model.NPUSwitchGroup
	}{
		{
			name: "real cluster HCCS group",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: realDeviceTopology,
			},
			want: []model.NPUSwitchGroup{
				{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}}},
			},
		},
		{
			name: "several groups of several types",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `{"topologys":[` +
					`{"type":"HCCS","groups":["0,1,2,3","4,5,6,7"]},` +
					`{"type":"SIO","groups":["0,1"]}]}`,
			},
			want: []model.NPUSwitchGroup{
				{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}}},
				{Type: "SIO", Groups: [][]int{{0, 1}}},
			},
		},
		{
			name: "non-numeric chip tokens are skipped",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `{"topologys":[{"type":"HCCS","groups":["0,x,2, 3 "]}]}`,
			},
			want: []model.NPUSwitchGroup{
				{Type: "HCCS", Groups: [][]int{{0, 2, 3}}},
			},
		},
		{
			name: "a group with no numeric token is dropped",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `{"topologys":[{"type":"HCCS","groups":["x,y"]}]}`,
			},
			want: nil,
		},
		{
			name: "a missing type keeps the usable groups",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `{"topologys":[{"groups":["0,1"]}]}`,
			},
			want: []model.NPUSwitchGroup{
				{Type: "", Groups: [][]int{{0, 1}}},
			},
		},
		{
			name: "a type drift to a number is stringified rather than dropped",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `{"topologys":[{"type":1,"groups":["0,1"]}]}`,
			},
			want: []model.NPUSwitchGroup{
				{Type: "1", Groups: [][]int{{0, 1}}},
			},
		},
		{
			name: "a numeric chip group is stringified and kept",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `{"topologys":[{"type":"HCCS","groups":[0]}]}`,
			},
			want: []model.NPUSwitchGroup{
				{Type: "HCCS", Groups: [][]int{{0}}},
			},
		},
		{
			name: "malformed JSON yields no topology",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `{"topologys":[`,
			},
			want: nil,
		},
		{
			name: "a JSON array instead of an object yields no topology",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: `[{"type":"HCCS"}]`,
			},
			want: nil,
		},
		{
			name: "empty annotation yields no topology",
			annotations: map[string]string{
				npuDeviceTopologyAnnotation: "   ",
			},
			want: nil,
		},
		{
			name:        "absent annotation yields no topology",
			annotations: map[string]string{"unrelated": "value"},
			want:        nil,
		},
		{
			name:        "nil annotations yield no topology",
			annotations: nil,
			want:        nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseNPUTopology(tc.annotations)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseNPUTopology() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseNPURankTable(t *testing.T) {
	// The real eight devices, as published by the live node.
	realEntries := []model.NPUDeviceNetEntry{
		{DeviceID: 0, DeviceIP: "29.11.129.127", TORIP: "29.79.161.163", TORPort: "18:1"},
		{DeviceID: 1, DeviceIP: "29.11.182.239", TORIP: "29.79.161.163", TORPort: "18:2"},
		{DeviceID: 2, DeviceIP: "29.11.159.1", TORIP: "29.79.161.169", TORPort: "18:1"},
		{DeviceID: 3, DeviceIP: "29.11.108.246", TORIP: "29.79.161.169", TORPort: "18:2"},
		{DeviceID: 4, DeviceIP: "29.11.73.127", TORIP: "29.79.161.169", TORPort: "17:2"},
		{DeviceID: 5, DeviceIP: "29.11.61.222", TORIP: "29.79.161.169", TORPort: "17:1"},
		{DeviceID: 6, DeviceIP: "29.11.45.20", TORIP: "29.79.161.163", TORPort: "17:2"},
		{DeviceID: 7, DeviceIP: "29.11.173.142", TORIP: "29.79.161.163", TORPort: "17:1"},
	}

	tests := []struct {
		name        string
		annotations map[string]string
		want        []model.NPUDeviceNetEntry
	}{
		{
			name: "real cluster rank table",
			annotations: map[string]string{
				npuRankTableAnnotation: realRankTable,
			},
			want: realEntries,
		},
		{
			name: "entries are sorted by device ID",
			annotations: map[string]string{
				npuRankTableAnnotation: `{"device":[` +
					`{"device_id":"7","device_ip":"10.0.0.7","tor_ip":"10.1.0.1","tor_port":"18:1"},` +
					`{"device_id":"2","device_ip":"10.0.0.2","tor_ip":"10.1.0.1","tor_port":"18:2"},` +
					`{"device_id":"5","device_ip":"10.0.0.5","tor_ip":"10.1.0.1","tor_port":"17:1"}]}`,
			},
			want: []model.NPUDeviceNetEntry{
				{DeviceID: 2, DeviceIP: "10.0.0.2", TORIP: "10.1.0.1", TORPort: "18:2"},
				{DeviceID: 5, DeviceIP: "10.0.0.5", TORIP: "10.1.0.1", TORPort: "17:1"},
				{DeviceID: 7, DeviceIP: "10.0.0.7", TORIP: "10.1.0.1", TORPort: "18:1"},
			},
		},
		{
			name: "a non-numeric device ID drops only that entry",
			annotations: map[string]string{
				npuRankTableAnnotation: `{"device":[` +
					`{"device_id":"not-a-number","device_ip":"10.0.0.1"},` +
					`{"device_id":"3","device_ip":"10.0.0.3","tor_ip":"10.1.0.1","tor_port":"18:1"}]}`,
			},
			want: []model.NPUDeviceNetEntry{
				{DeviceID: 3, DeviceIP: "10.0.0.3", TORIP: "10.1.0.1", TORPort: "18:1"},
			},
		},
		{
			name: "a missing device ID drops only that entry",
			annotations: map[string]string{
				npuRankTableAnnotation: `{"device":[` +
					`{"device_ip":"10.0.0.1"},` +
					`{"device_id":"4","device_ip":"10.0.0.4"}]}`,
			},
			want: []model.NPUDeviceNetEntry{
				{DeviceID: 4, DeviceIP: "10.0.0.4"},
			},
		},
		{
			name: "a numeric device ID drift is stringified and kept",
			annotations: map[string]string{
				npuRankTableAnnotation: `{"device":[{"device_id":6,"device_ip":"10.0.0.6"}]}`,
			},
			want: []model.NPUDeviceNetEntry{
				{DeviceID: 6, DeviceIP: "10.0.0.6"},
			},
		},
		{
			name: "a missing tor field keeps the entry",
			annotations: map[string]string{
				npuRankTableAnnotation: `{"device":[{"device_id":"1","device_ip":"10.0.0.1"}]}`,
			},
			want: []model.NPUDeviceNetEntry{
				{DeviceID: 1, DeviceIP: "10.0.0.1"},
			},
		},
		{
			name: "an empty device list yields nothing",
			annotations: map[string]string{
				npuRankTableAnnotation: `{"device":[]}`,
			},
			want: nil,
		},
		{
			name: "malformed JSON yields nothing",
			annotations: map[string]string{
				npuRankTableAnnotation: `{"device":[{"device_id":`,
			},
			want: nil,
		},
		{
			name: "an empty annotation yields nothing",
			annotations: map[string]string{
				npuRankTableAnnotation: "",
			},
			want: nil,
		},
		{
			name:        "absent annotation yields nothing",
			annotations: map[string]string{"unrelated": "value"},
			want:        nil,
		},
		{
			name:        "nil annotations yield nothing",
			annotations: nil,
			want:        nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseNPURankTable(tc.annotations)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseNPURankTable() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestConvertNodeAppliesNPUTopology(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "npu-node",
			Annotations: map[string]string{
				npuDeviceTopologyAnnotation: realDeviceTopology,
				npuRankTableAnnotation:      realRankTable,
			},
		},
	}

	nodeData := ConvertNode(node)

	wantTopology := []model.NPUSwitchGroup{
		{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}}},
	}
	if !reflect.DeepEqual(nodeData.NPUTopology, wantTopology) {
		t.Errorf("NPUTopology = %#v, want %#v", nodeData.NPUTopology, wantTopology)
	}

	if len(nodeData.NPUDeviceNet) != 8 {
		t.Fatalf("len(NPUDeviceNet) = %d, want 8", len(nodeData.NPUDeviceNet))
	}
	for i, entry := range nodeData.NPUDeviceNet {
		if entry.DeviceID != i {
			t.Errorf("entry %d has DeviceID %d, want ascending order", i, entry.DeviceID)
		}
	}
	if nodeData.NPUDeviceNet[0].DeviceIP != "29.11.129.127" {
		t.Errorf("device 0 IP = %q, want %q", nodeData.NPUDeviceNet[0].DeviceIP, "29.11.129.127")
	}
}

func TestConvertNodeWithoutTopologyAnnotations(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "plain-node"},
	}

	nodeData := ConvertNode(node)

	if nodeData.NPUTopology != nil {
		t.Errorf("NPUTopology = %#v, want nil", nodeData.NPUTopology)
	}
	if nodeData.NPUDeviceNet != nil {
		t.Errorf("NPUDeviceNet = %#v, want nil", nodeData.NPUDeviceNet)
	}
}

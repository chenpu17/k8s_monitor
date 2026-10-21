package datasource

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// Node annotations carrying NPU interconnect topology and per-device network
// placement. Both are published by the CCE node inspection pipeline on live
// Ascend nodes; a node that does not publish them simply leaves the
// corresponding model field nil.
const (
	// npuDeviceTopologyAnnotation carries the interconnect affinity groups,
	// e.g. {"topologys":[{"type":"HCCS","groups":["0,1,2,3,4,5,6,7"]}]}.
	npuDeviceTopologyAnnotation = "volcano.sh/device-topologys"

	// npuRankTableAnnotation carries the per-device RoCE placement, e.g.
	// {"device":[{"device_ip":"29.11.129.127","device_id":"0","tor_port":"18:1",
	// "tor_ip":"29.79.161.163"}, ...]}.
	npuRankTableAnnotation = "cce.kubectl.kubernetes.io/ascend-rank-table"
)

// npuTopologyJSON mirrors volcano.sh/device-topologys. Fields are decoded as
// interface{} so that a type drift on one field (for example a numeric chip
// group) degrades to an unknown value instead of failing the whole document.
type npuTopologyJSON struct {
	Topologys []struct {
		Type   interface{}   `json:"type"`
		Groups []interface{} `json:"groups"`
	} `json:"topologys"`
}

// npuRankTableJSON mirrors cce.kubectl.kubernetes.io/ascend-rank-table. Fields
// are decoded as interface{} for the same reason as npuTopologyJSON.
type npuRankTableJSON struct {
	Device []struct {
		DeviceIP interface{} `json:"device_ip"`
		DeviceID interface{} `json:"device_id"`
		TORPort  interface{} `json:"tor_port"`
		TORIP    interface{} `json:"tor_ip"`
	} `json:"device"`
}

// applyNPUTopology parses the NPU interconnect topology and rank-table
// annotations onto nodeData. Both are optional: absent, empty or malformed
// input leaves the corresponding field nil rather than being guessed.
func applyNPUTopology(nodeData *model.NodeData, annotations map[string]string) {
	nodeData.NPUTopology = parseNPUTopology(annotations)
	nodeData.NPUDeviceNet = parseNPURankTable(annotations)
}

// parseNPUTopology parses volcano.sh/device-topologys into switch groups. It
// returns nil when the annotation is absent, empty, not a JSON object, or
// carries no usable group.
func parseNPUTopology(annotations map[string]string) []model.NPUSwitchGroup {
	raw, ok := annotations[npuDeviceTopologyAnnotation]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}

	var doc npuTopologyJSON
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}

	topology := make([]model.NPUSwitchGroup, 0, len(doc.Topologys))
	for _, entry := range doc.Topologys {
		groups := make([][]int, 0, len(entry.Groups))
		for _, rawGroup := range entry.Groups {
			if chips := parseNPUChipList(jsonScalarString(rawGroup)); len(chips) > 0 {
				groups = append(groups, chips)
			}
		}
		if len(groups) == 0 {
			continue
		}
		// The type is descriptive only: keep the entry with an empty type
		// rather than dropping usable chip groups over a missing label.
		topology = append(topology, model.NPUSwitchGroup{
			Type:   jsonScalarString(entry.Type),
			Groups: groups,
		})
	}

	if len(topology) == 0 {
		return nil
	}
	return topology
}

// parseNPUChipList splits a comma-separated chip list such as "0,1,2,3,4,5,6,7"
// into chip IDs. Tokens that are not numbers are skipped, and an input with no
// numeric token yields nil.
func parseNPUChipList(raw string) []int {
	chips := make([]int, 0, 8)
	for _, token := range strings.Split(raw, ",") {
		chip, err := strconv.Atoi(strings.TrimSpace(token))
		if err != nil {
			continue
		}
		chips = append(chips, chip)
	}
	if len(chips) == 0 {
		return nil
	}
	return chips
}

// parseNPURankTable parses cce.kubectl.kubernetes.io/ascend-rank-table into
// per-device network entries sorted by device ID. It returns nil when the
// annotation is absent, empty, not a JSON object, or has no entry with a
// numeric device_id.
func parseNPURankTable(annotations map[string]string) []model.NPUDeviceNetEntry {
	raw, ok := annotations[npuRankTableAnnotation]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}

	var doc npuRankTableJSON
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}

	entries := make([]model.NPUDeviceNetEntry, 0, len(doc.Device))
	for _, device := range doc.Device {
		// device_id identifies the entry and is the sort key, so an entry
		// without a numeric one is dropped instead of being guessed.
		deviceID, err := strconv.Atoi(jsonScalarString(device.DeviceID))
		if err != nil {
			continue
		}
		entries = append(entries, model.NPUDeviceNetEntry{
			DeviceID: deviceID,
			DeviceIP: jsonScalarString(device.DeviceIP),
			TORIP:    jsonScalarString(device.TORIP),
			TORPort:  jsonScalarString(device.TORPort),
		})
	}

	if len(entries) == 0 {
		return nil
	}

	// The rank table is a map keyed by device_id and its order is not part of
	// the contract, so sort by device ID to keep the UI table stable.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].DeviceID < entries[j].DeviceID
	})
	return entries
}

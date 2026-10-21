package datasource

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// Node metadata keys carrying NPU health information.
const (
	// npuStatusAnnotation carries the device-plugin per-chip health bitmask,
	// e.g. "0xffffffffffffff00". The low 8 bits are per-chip flags (bit i =
	// davinci chip i, 1 = faulty), the upper 56 bits are padding always set to
	// 1, and an all-ones value is the startup state before the plugin reports
	// real chip health.
	npuStatusAnnotation     = "huawei.com/ascend-status"
	npuStatus1980Annotation = "huawei.com/ascend-1980-status"

	// npuFaultInfoAnnotation carries the management-plane fault document
	// (device / switch / custom fault levels and events).
	npuFaultInfoAnnotation = "os.modelarts.node/fault-info"
)

// npuStatusKeys is the lookup order for the health bitmask: the generic key
// first, then the Ascend 1980 variant. Both are checked as annotations first
// and then as labels, because different plugin versions publish one or the
// other.
var npuStatusKeys = []string{npuStatusAnnotation, npuStatus1980Annotation}

// applyNPUHealth parses NPU health metadata from node annotations/labels onto
// nodeData. Missing, empty or malformed values leave the corresponding fields
// unset (unknown) instead of being guessed.
func applyNPUHealth(nodeData *model.NodeData, annotations, labels map[string]string) {
	if raw, ok := lookupNPUStatus(annotations, labels); ok {
		nodeData.NPUStatusMaskRaw = raw
		// ParseUint with base 0 accepts the "0x" prefix the plugin uses as well
		// as a bare hex/decimal value; anything else leaves the mask unknown.
		if mask, err := strconv.ParseUint(raw, 0, 64); err == nil && mask != math.MaxUint64 {
			nodeData.NPUStatusMask = mask
			nodeData.NPUStatusKnown = true
		}
	}

	nodeData.NPUFault = parseNPUFaultInfo(annotations)
}

// lookupNPUStatus returns the first non-empty status bitmask found in
// annotations (preferred) or labels.
func lookupNPUStatus(annotations, labels map[string]string) (string, bool) {
	for _, source := range []map[string]string{annotations, labels} {
		for _, key := range npuStatusKeys {
			value, ok := source[key]
			if !ok {
				continue
			}
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed, true
			}
		}
	}
	return "", false
}

// npuFaultInfoJSON mirrors the os.modelarts.node/fault-info document. Fields are
// decoded as interface{} so that a type drift (for example a numeric fault
// level) degrades to an unknown value instead of failing the whole document.
type npuFaultInfoJSON struct {
	SwitchFault struct {
		SwitchName interface{} `json:"SwitchName"`
		FaultLevel interface{} `json:"FaultLevel"`
		FaultEvent interface{} `json:"FaultEvent"`
	} `json:"SwitchFault"`
	DeviceFault struct {
		FaultLevel interface{} `json:"FaultLevel"`
		FaultEvent interface{} `json:"FaultEvent"`
	} `json:"DeviceFault"`
	CustomFault struct {
		FaultLevel interface{} `json:"FaultLevel"`
	} `json:"CustomFault"`
}

// parseNPUFaultInfo parses the management-plane fault annotation. It returns
// nil when the annotation is absent, empty or not a JSON object, so callers can
// distinguish "no fault information" from "no fault".
func parseNPUFaultInfo(annotations map[string]string) *model.NPUFaultInfo {
	raw, ok := annotations[npuFaultInfoAnnotation]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}

	var doc npuFaultInfoJSON
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}

	return &model.NPUFaultInfo{
		DeviceFaultLevel:  faultString(doc.DeviceFault.FaultLevel),
		DeviceFaultEvents: faultEventList(doc.DeviceFault.FaultEvent),
		SwitchFaultLevel:  faultString(doc.SwitchFault.FaultLevel),
		SwitchFaultName:   faultString(doc.SwitchFault.SwitchName),
		CustomFaultLevel:  faultString(doc.CustomFault.FaultLevel),
	}
}

// faultString converts a decoded JSON value to a trimmed string. Numbers and
// booleans are stringified so that an unexpected type still surfaces a value
// (which callers treat as an unknown fault level) instead of disappearing.
func faultString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

// faultEventList normalises the FaultEvent field, which the cluster reports as
// an array whose elements may be plain strings or objects.
func faultEventList(value interface{}) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []interface{}:
		events := make([]string, 0, len(typed))
		for _, item := range typed {
			if event := faultEventString(item); event != "" {
				events = append(events, event)
			}
		}
		if len(events) == 0 {
			return nil
		}
		return events
	default:
		if event := faultEventString(value); event != "" {
			return []string{event}
		}
		return nil
	}
}

// faultEventString renders a single fault event element.
func faultEventString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}, []interface{}:
		// Compact re-encoding keeps object events readable in one line.
		encoded, err := json.Marshal(typed)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(typed))
		}
		return string(encoded)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

// npuAlertMessageMaxLen bounds the free-form text (fault events, inspection
// messages) copied into alert messages.
const npuAlertMessageMaxLen = 200

// npuCheckConditionPrefix selects the node conditions that are NPU fault
// checks. Only the NT_NPU_* family raises npu_check_failed alerts; other NT_*
// conditions are inspection plumbing that can be True on a perfectly healthy
// node (see the comment at the alert site in aggregated.go).
const npuCheckConditionPrefix = "NT_NPU_"

// npuDeviceFaultSeverity maps a management-plane device fault level to an alert
// severity. The second return value is false for levels that must not raise an
// alert: an empty level means the field was not reported, and NormalNPU means
// no fault. Any other (unknown) level maps to Warning so that a level this
// build does not know about is never silently swallowed.
func npuDeviceFaultSeverity(level string) (model.AlertSeverity, bool) {
	switch level {
	case "", "NormalNPU":
		return model.AlertSeverityInfo, false
	case "UrgentNPU":
		return model.AlertSeverityCritical, true
	case "ImportantNPU", "WarningNPU", "NotHandle":
		return model.AlertSeverityWarning, true
	default:
		return model.AlertSeverityWarning, true
	}
}

// sanitizeNPUText collapses all whitespace (including the newlines some
// inspection messages embed) and truncates to maxLen runes.
func sanitizeNPUText(text string, maxLen int) string {
	collapsed := strings.Join(strings.Fields(text), " ")
	runes := []rune(collapsed)
	if len(runes) <= maxLen {
		return collapsed
	}
	return string(runes[:maxLen]) + "..."
}

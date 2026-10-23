package datasource

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/chenpu/k8s-monitor/internal/diagnostic"
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
		DeviceFaultLevel:  jsonScalarString(doc.DeviceFault.FaultLevel),
		DeviceFaultEvents: faultEventList(doc.DeviceFault.FaultEvent),
		SwitchFaultLevel:  jsonScalarString(doc.SwitchFault.FaultLevel),
		SwitchFaultName:   jsonScalarString(doc.SwitchFault.SwitchName),
		CustomFaultLevel:  jsonScalarString(doc.CustomFault.FaultLevel),
	}
}

// jsonScalarString converts a decoded JSON scalar to a trimmed string. Numbers
// and booleans are stringified so that an unexpected type still surfaces a
// value (which callers treat as an unknown field) instead of disappearing.
func jsonScalarString(value interface{}) string {
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

// NPU per-chip thresholds evaluated against NPU-Exporter metrics.
//
// The temperature thresholds are conservative empirical values for the Ascend
// 910B family: live 910B1 chips sit at 46-49°C idle, and no official Tjmax/TDP
// figure was available to derive them from. They should become configurable
// once a vendor number is published.
const (
	npuTempWarningCelsius  = 80
	npuTempCriticalCelsius = 90
)

// npuThresholdAlerts builds the per-chip threshold alerts for one node from the
// NPU-Exporter metrics. Each alert type is emitted at most once per node, with
// the offending chip IDs aggregated into Value ("chips 0, 3"), so a node whose
// eight chips are all hot still produces one temperature alert rather than
// eight. It returns nil when the node carries no per-chip metrics, which is
// also the case when the NPU-Exporter is unavailable.
//
// As a side effect it fills node.NPUChipHealth: the same per-chip checks that
// raise the alerts also grade every chip, because the chip attribution of an
// alert otherwise only survives as free text in Value ("chips 0, 3") and the UI
// cannot recover it reliably. The field stays nil when the node has no chip
// metrics, which is the same presence rule this function uses to decide whether
// there is anything to alert on.
//
// Deliberately not alerted on here:
//   - power: no authoritative TDP is published for the 910B family, so any
//     wattage threshold would be invented.
//   - RoCE error packets: the counters are cumulative and this build has no
//     rate baseline, so a residual non-zero value left over from an earlier
//     incident would be reported as an ongoing fault.
func npuThresholdAlerts(node *model.NodeData, now time.Time) []model.Alert {
	if node == nil || len(node.NPUChips) == 0 {
		return nil
	}

	var (
		hotChips    []string
		maxTemp     int
		eccDouble   []string
		eccSingle   []string
		linkDown    []string
		errorChips  []string
		errorDetail []string
	)

	grades := newNPUChipGrades(node.NPUChips)

	for i, chip := range node.NPUChips {
		id := strconv.Itoa(chip.PhyID)

		if chip.Temp >= npuTempWarningCelsius {
			hotChips = append(hotChips, id)
			if chip.Temp > maxTemp {
				maxTemp = chip.Temp
			}
			// Temperature follows the severity split of the alert it produces:
			// the 90°C line is the critical one, so a chip above it is graded
			// fault rather than degraded.
			if chip.Temp >= npuTempCriticalCelsius {
				grades.hit(i, model.NPUChipGradeFault, fmt.Sprintf("temp %d>=%d", chip.Temp, npuTempCriticalCelsius))
			} else {
				grades.hit(i, model.NPUChipGradeDegraded, fmt.Sprintf("temp %d>=%d", chip.Temp, npuTempWarningCelsius))
			}
		}

		switch {
		case chip.HBMEccDoubleErr > 0:
			eccDouble = append(eccDouble, id)
			grades.hit(i, model.NPUChipGradeFault, "double-bit ECC")
		case chip.HBMEccSingleErr > 0:
			// Single-bit errors are correctable and are only surfaced for chips
			// that have no double-bit error: those are already covered by the
			// critical alert above, and repeating them here would add noise.
			eccSingle = append(eccSingle, id)
			grades.hit(i, model.NPUChipGradeDegraded, "single-bit ECC")
		}

		// LinkStatus is only meaningful when the exporter actually reported it:
		// a chip whose link_status metric is missing keeps the zero value, which
		// must not be read as "link down".
		if chip.LinkStatusKnown && chip.LinkStatus == 0 {
			linkDown = append(linkDown, id)
			grades.hit(i, model.NPUChipGradeFault, "link down")
		}

		if chip.ErrorCode != 0 {
			errorChips = append(errorChips, id)
			errorDetail = append(errorDetail, fmt.Sprintf("%s (code %d)", id, chip.ErrorCode))
			grades.hit(i, model.NPUChipGradeFault, fmt.Sprintf("error code %d", chip.ErrorCode))
		}

		// The exporter-reported health string is a second, independent signal
		// next to the metrics above; it only ever raises a chip's grade.
		switch chip.Health {
		case "Error":
			grades.hit(i, model.NPUChipGradeFault, "chip health Error")
		case "Warning":
			grades.hit(i, model.NPUChipGradeDegraded, "chip health Warning")
		}
	}

	// Node-level NPU fault signals grade every chip of the node: neither the
	// device-plugin bitmask nor the management-plane fault level is attributable
	// to a single chip from the data at hand.
	grades.hitNodeWide(node)

	node.NPUChipHealth = grades.results()

	alerts := make([]model.Alert, 0, 5)

	if len(hotChips) > 0 {
		// Severity follows the hottest chip; Value lists every chip above the
		// warning threshold. Threshold reports the line matching the severity,
		// so a Critical alert does not advertise the 80°C warning line.
		severity := model.AlertSeverityWarning
		reportedThreshold := npuTempWarningCelsius
		if maxTemp >= npuTempCriticalCelsius {
			severity = model.AlertSeverityCritical
			reportedThreshold = npuTempCriticalCelsius
		}
		alerts = append(alerts, model.Alert{
			Severity:          severity,
			Category:          "NPU",
			AlertType:         model.AlertTypeNPUTempHigh,
			ResourceType:      "Node",
			ResourceName:      node.Name,
			Message:           fmt.Sprintf("NPU temperature high on chip(s) %s (max %d°C)", strings.Join(hotChips, ", "), maxTemp),
			MessageKey:        "alert.npu_temp_high",
			MessageArgs:       map[string]interface{}{"Chips": strings.Join(hotChips, ", "), "Max": maxTemp},
			Value:             npuChipValue(hotChips),
			ValueKey:          "alert.value_chips",
			ValueArgs:         map[string]interface{}{"Chips": strings.Join(hotChips, ", ")},
			Threshold:         fmt.Sprintf("%d°C", reportedThreshold),
			RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNPUTempHigh, "", node.Name),
			Timestamp:         now,
		})
	}

	// ECC alerts read the per-window _cnt family (npu_chip_info_hbm_ecc_*_bit_error_cnt),
	// not the cumulative npu_chip_info_hbm_ecc_total_*_bit_error_cnt family:
	// a chip that accumulated corrections in the past but sees no new ones is
	// not an ongoing fault. The totals are parsed (model NPUChipData
	// *ErrTotal) so the UI can show long-term HBM degradation instead. On the
	// reference node one chip reports _cnt=0 with a six-digit cumulative total,
	// which must not raise an alert.
	if len(eccDouble) > 0 {
		alerts = append(alerts, model.Alert{
			Severity:          model.AlertSeverityCritical,
			Category:          "NPU",
			AlertType:         model.AlertTypeNPUEccDouble,
			ResourceType:      "Node",
			ResourceName:      node.Name,
			Message:           fmt.Sprintf("NPU HBM double-bit ECC error(s) on chip(s) %s (uncorrectable)", strings.Join(eccDouble, ", ")),
			MessageKey:        "alert.npu_ecc_double_bit",
			MessageArgs:       map[string]interface{}{"Chips": strings.Join(eccDouble, ", ")},
			Value:             npuChipValue(eccDouble),
			ValueKey:          "alert.value_chips",
			ValueArgs:         map[string]interface{}{"Chips": strings.Join(eccDouble, ", ")},
			RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNPUEccDouble, "", node.Name),
			Timestamp:         now,
		})
	}

	if len(eccSingle) > 0 {
		alerts = append(alerts, model.Alert{
			Severity:          model.AlertSeverityWarning,
			Category:          "NPU",
			AlertType:         model.AlertTypeNPUEccSingle,
			ResourceType:      "Node",
			ResourceName:      node.Name,
			Message:           fmt.Sprintf("NPU HBM single-bit ECC error(s) on chip(s) %s (correctable)", strings.Join(eccSingle, ", ")),
			MessageKey:        "alert.npu_ecc_single_bit",
			MessageArgs:       map[string]interface{}{"Chips": strings.Join(eccSingle, ", ")},
			Value:             npuChipValue(eccSingle),
			ValueKey:          "alert.value_chips",
			ValueArgs:         map[string]interface{}{"Chips": strings.Join(eccSingle, ", ")},
			RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNPUEccSingle, "", node.Name),
			Timestamp:         now,
		})
	}

	if len(linkDown) > 0 {
		alerts = append(alerts, model.Alert{
			Severity:          model.AlertSeverityCritical,
			Category:          "NPU",
			AlertType:         model.AlertTypeNPULinkDown,
			ResourceType:      "Node",
			ResourceName:      node.Name,
			Message:           fmt.Sprintf("NPU link down on chip(s) %s", strings.Join(linkDown, ", ")),
			MessageKey:        "alert.npu_link_down",
			MessageArgs:       map[string]interface{}{"Chips": strings.Join(linkDown, ", ")},
			Value:             npuChipValue(linkDown),
			ValueKey:          "alert.value_chips",
			ValueArgs:         map[string]interface{}{"Chips": strings.Join(linkDown, ", ")},
			RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNPULinkDown, "", node.Name),
			Timestamp:         now,
		})
	}

	if len(errorChips) > 0 {
		// The error code semantics are not documented, so this stays a warning
		// and the raw code is carried through in the message for whoever does
		// know it. The value column keeps the plain chip IDs (the message
		// carries the raw codes), so the en render stays byte-identical to
		// Value.
		alerts = append(alerts, model.Alert{
			Severity:          model.AlertSeverityWarning,
			Category:          "NPU",
			AlertType:         model.AlertTypeNPUErrorCode,
			ResourceType:      "Node",
			ResourceName:      node.Name,
			Message:           fmt.Sprintf("NPU reported error code on chip(s) %s", strings.Join(errorDetail, ", ")),
			MessageKey:        "alert.npu_error_code",
			MessageArgs:       map[string]interface{}{"Chips": strings.Join(errorDetail, ", ")},
			Value:             npuChipValue(errorChips),
			ValueKey:          "alert.value_chips",
			ValueArgs:         map[string]interface{}{"Chips": strings.Join(errorChips, ", ")},
			RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNPUErrorCode, "", node.Name),
			Timestamp:         now,
		})
	}

	return alerts
}

// npuChipValue renders chip IDs in the Value format shared with the device
// plugin chip-fault alert, e.g. "chips 0, 3".
func npuChipValue(chips []string) string {
	return "chips " + strings.Join(chips, ", ")
}

// npuChipGradeRanks orders the chip grades so a chip hit by several checks keeps
// the most severe one.
var npuChipGradeRanks = map[string]int{
	model.NPUChipGradeHealthy:  0,
	model.NPUChipGradeDegraded: 1,
	model.NPUChipGradeFault:    2,
}

// npuChipGrades accumulates one grade per chip while the per-chip checks run.
// Grades are ranked highest-wins and the reason is the one of the trigger that
// set that grade, the first one in detection order when several triggers of the
// same grade fire.
type npuChipGrades struct {
	chips  []model.NPUChipData
	grades []string
	reason []string
}

// newNPUChipGrades starts the accumulator with every chip healthy, in the order
// the node reports its chips (the canonical order of NPUChipHealth).
func newNPUChipGrades(chips []model.NPUChipData) *npuChipGrades {
	g := &npuChipGrades{
		chips:  chips,
		grades: make([]string, len(chips)),
		reason: make([]string, len(chips)),
	}
	for i := range g.grades {
		g.grades[i] = model.NPUChipGradeHealthy
	}
	return g
}

// hit raises chip i's grade to grade with the given reason. A lower grade never
// overwrites a higher one, and an equal grade keeps the reason that fired first
// so the output does not depend on the order the checks are evaluated in.
func (g *npuChipGrades) hit(i int, grade, reason string) {
	if i < 0 || i >= len(g.grades) {
		return
	}
	switch {
	case npuChipGradeRanks[grade] > npuChipGradeRanks[g.grades[i]]:
		g.grades[i] = grade
		g.reason[i] = reason
	case npuChipGradeRanks[grade] == npuChipGradeRanks[g.grades[i]] && g.reason[i] == "":
		g.reason[i] = reason
	}
}

// hitNodeWide grades every chip of the node with signals that are not
// attributable to a single chip: the device plugin's per-chip fault bitmask
// (which does name a chip, see below) and the management-plane device fault
// level (which does not).
func (g *npuChipGrades) hitNodeWide(node *model.NodeData) {
	// The device plugin bitmask does name the chip: bit i is davinci chip i,
	// which is the chip's PhyID. Chips outside the bitmap's range keep whatever
	// the exporter metrics said about them.
	if node.NPUStatusKnown {
		for i, chip := range g.chips {
			if node.NPUChipFaulty(chip.PhyID) {
				g.hit(i, model.NPUChipGradeFault, "device plugin fault bit")
			}
		}
	}

	// A management-plane device fault is reported for the NPU subsystem as a
	// whole, so it grades every chip of the node. Switch faults are deliberately
	// left out: a faulty interconnect switch is not a fault of the chips behind
	// it, and the alert for it still surfaces in the alerts view.
	if fault := node.NPUFault; fault != nil {
		severity, report := npuDeviceFaultSeverity(fault.DeviceFaultLevel)
		if report {
			grade := model.NPUChipGradeDegraded
			if severity == model.AlertSeverityCritical {
				grade = model.NPUChipGradeFault
			}
			for i := range g.chips {
				g.hit(i, grade, "device fault level "+fault.DeviceFaultLevel)
			}
		}
	}
}

// results returns the accumulated grades as the model slice. The reason of a
// healthy chip stays empty: there is nothing to report about it.
func (g *npuChipGrades) results() []model.NPUChipGrade {
	out := make([]model.NPUChipGrade, len(g.chips))
	for i, chip := range g.chips {
		out[i] = model.NPUChipGrade{
			NPUID:  chip.NPUID,
			Chip:   chip.Chip,
			Grade:  g.grades[i],
			Reason: g.reason[i],
		}
	}
	return out
}

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

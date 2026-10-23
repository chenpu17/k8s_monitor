package datasource

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// NPU fault-code knowledge base.
//
// faultCode.json is a byte-for-byte copy of the file Ascend Device Plugin ships
// in its image (`component/ascend-device-plugin/build/faultCode.json`), which
// the plugin uses to classify the error code a chip reports into one of eight
// handling levels. Snapshot: mind-cluster master as published on 2026-10-23,
// Apache-2.0. The copy is embedded rather than fetched so the lookup works
// offline and cannot change shape under a running monitor.
//
// Byte counts of the snapshot: 8 levels holding 804 raw entries, which decode to
// 802 distinct codes (B4060011 is listed twice inside RestartBusinessCodes and
// A2301001 is listed by both RestartBusinessCodes and SeparateNPUCodes). The
// first level wins for a code listed twice — see npuErrorCodeSnapshot.
//
//	NotHandleFaultCodes   132
//	RestartRequestCodes    33
//	RestartBusinessCodes   26
//	FreeRestartNPUCodes    24
//	RestartNPUCodes       161
//	SeparateNPUCodes      215
//	PreSeparateNPUCodes   200
//	SubHealthFaultCodes    13
//
//go:embed faultCode.json
var npuFaultCodeJSON []byte

// faultCodeDoc mirrors faultCode.json: one level per key, each a list of codes
// written as uppercase hex strings. Codes are decoded leniently (see
// npuErrorCodeSnapshot) because five of the upstream entries carry an extra
// leading zero.
type faultCodeDoc struct {
	NotHandleFaultCodes  []string `json:"NotHandleFaultCodes"`
	RestartRequestCodes  []string `json:"RestartRequestCodes"`
	RestartBusinessCodes []string `json:"RestartBusinessCodes"`
	FreeRestartNPUCodes  []string `json:"FreeRestartNPUCodes"`
	RestartNPUCodes      []string `json:"RestartNPUCodes"`
	SeparateNPUCodes     []string `json:"SeparateNPUCodes"`
	PreSeparateNPUCodes  []string `json:"PreSeparateNPUCodes"`
	SubHealthFaultCodes  []string `json:"SubHealthFaultCodes"`
}

// npuErrorCodeLevels is the level -> handling-tier mapping the knowledge base
// exposes, in the order the upstream document lists them. The Category and
// Action phrases are the ones the data layer embeds in chip grades and alert
// actions; they are stable English identifiers, not UI copy, and are kept free
// of < > & so a JSON export of a grade reason keeps them verbatim.
var npuErrorCodeLevels = []struct {
	level    string
	codes    func(faultCodeDoc) []string
	category model.NPUErrorCodeCategory
}{
	{
		level: "NotHandleFault",
		codes: func(d faultCodeDoc) []string { return d.NotHandleFaultCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatNoAction,
			Category: "no action needed",
			Action:   "No action required; the fault does not affect workloads",
			Level:    "NotHandleFault",
		},
	},
	{
		level: "RestartRequest",
		codes: func(d faultCodeDoc) []string { return d.RestartRequestCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatRerun,
			Category: "workload rerun",
			Action:   "Rerun the workload; the chip recovers without reset",
			Level:    "RestartRequest",
		},
	},
	{
		level: "RestartBusiness",
		codes: func(d faultCodeDoc) []string { return d.RestartBusinessCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatRerun,
			Category: "workload rerun",
			Action:   "Rerun the workload; the chip recovers without reset",
			Level:    "RestartBusiness",
		},
	},
	{
		level: "FreeRestartNPU",
		codes: func(d faultCodeDoc) []string { return d.FreeRestartNPUCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatReset,
			Category: "NPU reset required",
			Action:   "Stop jobs on the chip and reset the NPU (npu-smi; FreeRestart waits for idle)",
			Level:    "FreeRestartNPU",
		},
	},
	{
		level: "RestartNPU",
		codes: func(d faultCodeDoc) []string { return d.RestartNPUCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatReset,
			Category: "NPU reset required",
			Action:   "Stop jobs on the chip and reset the NPU (npu-smi; FreeRestart waits for idle)",
			Level:    "RestartNPU",
		},
	},
	{
		level: "SeparateNPU",
		codes: func(d faultCodeDoc) []string { return d.SeparateNPUCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatIsolate,
			Category: "chip isolation required",
			Action:   "The chip is unrecoverable; isolate it and evaluate hardware replacement",
			Level:    "SeparateNPU",
		},
	},
	{
		level: "PreSeparateNPU",
		codes: func(d faultCodeDoc) []string { return d.PreSeparateNPUCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatIsolate,
			Category: "chip isolation required",
			Action:   "The chip is unrecoverable; isolate it and evaluate hardware replacement",
			Level:    "PreSeparateNPU",
		},
	},
	{
		level: "SubHealthFault",
		codes: func(d faultCodeDoc) []string { return d.SubHealthFaultCodes },
		category: model.NPUErrorCodeCategory{
			Family:   model.NPUErrorCatSubHealth,
			Category: "sub-health",
			Action:   "Chip is sub-healthy; apply the sub-health strategy and schedule accordingly",
			Level:    "SubHealthFault",
		},
	},
}

var (
	npuErrorCodeOnce sync.Once
	npuErrorCodeMap  map[int64]model.NPUErrorCodeCategory
)

// npuErrorCodeSnapshot returns the code -> handling-tier map, building it on
// first use. A code listed by two levels keeps the first level in
// npuErrorCodeLevels order (NotHandleFault first, the most permissive) and the
// duplicate is logged once: the file is static vendor data and a build-time
// panic over a duplicated entry would take the whole monitor down for a data
// quirk.
func npuErrorCodeSnapshot() map[int64]model.NPUErrorCodeCategory {
	npuErrorCodeOnce.Do(func() {
		npuErrorCodeMap = buildNPUErrorCodeMap(npuFaultCodeJSON, zap.L())
	})
	return npuErrorCodeMap
}

// buildNPUErrorCodeMap decodes the embedded document into the lookup map. It is
// split out so a test can feed a synthetic document, and it never panics: a
// document that does not decode yields an empty map and the callers fall back
// to their generic wording.
func buildNPUErrorCodeMap(raw []byte, logger *zap.Logger) map[int64]model.NPUErrorCodeCategory {
	var doc faultCodeDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		if logger != nil {
			logger.Warn("NPU fault-code knowledge base does not decode, error-code classification disabled",
				zap.Error(err))
		}
		return map[int64]model.NPUErrorCodeCategory{}
	}

	entries := make(map[int64]model.NPUErrorCodeCategory, 1024)
	for _, level := range npuErrorCodeLevels {
		for _, hex := range level.codes(doc) {
			code, err := strconv.ParseInt(hex, 16, 64)
			if err != nil {
				if logger != nil {
					logger.Warn("NPU fault-code knowledge base carries an undecodable code",
						zap.String("level", level.level), zap.String("code", hex))
				}
				continue
			}
			if seen, dup := entries[code]; dup {
				if logger != nil {
					logger.Warn("NPU fault-code knowledge base lists a code twice, keeping the first level",
						zap.String("code", npuErrorCodeHex(code)),
						zap.String("kept", seen.Level),
						zap.String("dropped", level.level),
					)
				}
				continue
			}
			entries[code] = level.category
		}
	}
	return entries
}

// npuErrorCodeHex renders an error code the way the knowledge base keys it:
// uppercase hex, zero padded to 8 digits ("80E21007").
func npuErrorCodeHex(code int64) string {
	return fmt.Sprintf("%08X", code)
}

// npuErrorCodeInfo looks an exporter error code up in the knowledge base. It
// returns false for code 0 (no error), for negative codes and for codes the
// snapshot does not carry, so callers can fall back to their generic wording
// instead of inventing a handling tier.
func npuErrorCodeInfo(code int64) (model.NPUErrorCodeCategory, bool) {
	return lookupNPUErrorCode(npuErrorCodeSnapshot(), code)
}

// lookupNPUErrorCode is the lookup itself, split out so the boundary rules can
// be probed against a synthetic table: it must refuse 0 and negative codes even
// when such an entry exists, because "no error" is never a fault class.
func lookupNPUErrorCode(table map[int64]model.NPUErrorCodeCategory, code int64) (model.NPUErrorCodeCategory, bool) {
	if code <= 0 {
		return model.NPUErrorCodeCategory{}, false
	}
	category, ok := table[code]
	return category, ok
}

// npuErrorCodeReason renders the classification the chip grade carries for a
// known error code, e.g. "error code 80E21007 (no action needed)". The reason
// is exported as JSON, so it must stay free of the characters encoding/json
// escapes (< > &); the category phrases are chosen accordingly.
func npuErrorCodeReason(code int64, category model.NPUErrorCodeCategory) string {
	return fmt.Sprintf("error code %s (%s)", npuErrorCodeHex(code), category.Category)
}

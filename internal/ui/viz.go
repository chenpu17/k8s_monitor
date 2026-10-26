package ui

import (
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The session series behind the sparklines: one sample per refresh that
// actually carried new metrics (the recorder is called from the same guarded
// path as recordMetricSnapshot). Cluster-level aggregates only — the per-node
// maps in metricHistory answer different questions and keep a different depth.

// recordVizSample appends one sample of the cluster-wide series and trims the
// ring to maxVizHistory.
func (m *Model) recordVizSample(data *model.ClusterData, at time.Time) {
	if data == nil {
		return
	}
	m.vizHistory = append(m.vizHistory, m.buildVizSample(data, at))
	if len(m.vizHistory) > m.maxVizHistory {
		m.vizHistory = m.vizHistory[len(m.vizHistory)-m.maxVizHistory:]
	}
}

// buildVizSample aggregates one cluster-wide sample out of the refresh payload.
func (m *Model) buildVizSample(data *model.ClusterData, at time.Time) VizSample {
	sample := VizSample{At: at}

	// CPU and memory: cluster totals from the summary when it is there, summed
	// from the nodes otherwise. Percentages are against capacity — the same
	// denominator the cluster-load box prints on the line the sparkline
	// attaches to, so a trend and its headline number cannot drift apart. (The
	// box's secondary line shows the allocatable variant; that is a different
	// question and not this series.)
	var cpuUsed, cpuCap, memUsed, memCap int64
	if s := data.Summary; s != nil {
		cpuUsed, cpuCap = s.CPUUsed, s.CPUCapacity
		memUsed, memCap = s.MemoryUsed, s.MemoryCapacity
	} else {
		for _, node := range data.Nodes {
			cpuUsed += node.CPUUsage
			cpuCap += node.CPUCapacity
			memUsed += node.MemoryUsage
			memCap += node.MemoryCapacity
		}
	}
	if cpuCap > 0 {
		sample.CPUPct = float64(cpuUsed) / float64(cpuCap) * 100
	}
	if memCap > 0 {
		sample.MemPct = float64(memUsed) / float64(memCap) * 100
	}

	// Network rates come from the same sliding-window helpers the cluster-load
	// box prints with, so the sparkline and the number beside it cannot
	// disagree.
	rxRate, txRate := m.calculateClusterNetworkRate()
	sample.NetRxKBps = rxRate * 1024
	sample.NetTxKBps = txRate * 1024

	// NPU aggregates over the chips that actually carry data. A chip counts as
	// reporting utilisation unless its exporter health status says it said
	// nothing ("Unknown"), which is the same rule the summary bar counts unknown
	// chips by. Chips with no data at all contribute nothing: an unknown
	// utilisation must not be averaged in as an idle one.
	utilSum, utilCount := 0.0, 0
	tempSum, chipCount := 0.0, 0
	for _, node := range data.Nodes {
		for _, chip := range node.NPUChips {
			chipCount++
			if chip.Health != "Unknown" {
				utilSum += float64(chip.AICore)
				utilCount++
			}
			tempSum += float64(chip.Temp)
			sample.NPUPowerW += chip.Power
			sample.HBMUsedMB += chip.HBMUsed
			sample.HBMTotalMB += chip.HBMTotal
		}
	}
	if utilCount > 0 {
		sample.NPUUtilKnown = true
		sample.NPUUtilPct = utilSum / float64(utilCount)
	}
	if chipCount > 0 {
		sample.NPUTempAvg = tempSum / float64(chipCount)
	}

	return sample
}

// currentVizSample measures the live clusterData exactly as a recorded sample
// would. It is the value half of the "same source, same value" contract: the
// numbers on screen and the series the sparkline is built from come out of this
// one function, so they cannot drift apart.
func (m *Model) currentVizSample() *VizSample {
	if m.clusterData == nil {
		return nil
	}
	sample := m.buildVizSample(m.clusterData, time.Time{})
	return &sample
}

// vizSeries returns the values of the session series picked by sel, in
// chronological order. Samples sel rejects are skipped, so a gap in the data
// shows as a gap instead of as a zero.
func (m *Model) vizSeries(sel func(VizSample) (float64, bool)) []float64 {
	vals := make([]float64, 0, len(m.vizHistory))
	for _, sample := range m.vizHistory {
		if v, ok := sel(sample); ok {
			vals = append(vals, v)
		}
	}
	return vals
}

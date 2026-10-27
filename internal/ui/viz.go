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

// The per-node series behind the node detail's sparklines. It follows the same
// guarded recording path as the cluster series: one sample per refresh that
// actually carried new metrics, never twice for the same snapshot.

// recordNodeVizSample appends one sample of node's session series and trims the
// ring to maxNodeVizHistory.
//
// A change of node starts a new session: the user switching from one node's
// detail to another would otherwise see one node's trend continued with the
// other's values, which is a shape neither node ever had. Leaving and
// re-entering the same node keeps the session.
func (m *Model) recordNodeVizSample(node *model.NodeData, at time.Time) {
	if node == nil {
		return
	}
	if m.nodeVizNodeName != node.Name {
		m.nodeVizHistory = m.nodeVizHistory[:0]
		m.nodeVizNodeName = node.Name
	}
	m.nodeVizHistory = append(m.nodeVizHistory, buildNodeVizSample(node, at))
	if len(m.nodeVizHistory) > m.maxNodeVizHistory {
		m.nodeVizHistory = m.nodeVizHistory[len(m.nodeVizHistory)-m.maxNodeVizHistory:]
	}
}

// nodeVizDataValid reports whether a node carries kubelet usage at all. The
// explicit flag is authoritative (the datasource zeroes the usage fields when
// the kubelet fetch fails); a non-zero usage is accepted as evidence too, so a
// payload that predates the flag or a fixture that omits it is not drawn as a
// missing metric.
func nodeVizDataValid(node *model.NodeData) bool {
	return node.HasKubeletMetrics || node.CPUUsage > 0 || node.MemoryUsage > 0
}

// buildNodeVizSample aggregates one per-node sample out of a refresh payload.
//
// CPU and memory are divided by the node's capacity — the denominator the node
// detail's headline percentages use — never by allocatable, so a sparkline and
// the number beside it cannot come from two different denominators. A missing
// kubelet summary leaves MemValid false instead of recording a fabricated zero;
// CPUPct stays zero and the sparkline renders it as the gap it is.
func buildNodeVizSample(node *model.NodeData, at time.Time) NodeVizSample {
	sample := NodeVizSample{At: at}
	if node == nil {
		return sample
	}

	if nodeVizDataValid(node) {
		if node.CPUCapacity > 0 {
			sample.CPUPct = float64(node.CPUUsage) / float64(node.CPUCapacity) * 100
		}
		if node.MemoryCapacity > 0 {
			sample.MemPct = float64(node.MemoryUsage) / float64(node.MemoryCapacity) * 100
			sample.MemValid = true
		}
	}

	// NPU aggregates over the chips that actually reported. A chip whose health
	// says "Unknown" said nothing about its utilisation, so it is not averaged
	// in as an idle chip — the same rule the cluster series follows. Power and
	// HBM are sums over every chip: a chip with unknown health still draws power
	// and occupies HBM capacity.
	utilSum, utilCount := 0.0, 0
	var hbmUsed, hbmTotal int64
	for _, chip := range node.NPUChips {
		if chip.Health != "Unknown" {
			utilSum += float64(chip.AICore)
			utilCount++
		}
		sample.NPUPowerW += chip.Power
		hbmUsed += chip.HBMUsed
		hbmTotal += chip.HBMTotal
	}
	if utilCount > 0 {
		sample.NPUUtilValid = true
		sample.NPUUtilPct = utilSum / float64(utilCount)
	}
	if hbmTotal > 0 {
		sample.HBMValid = true
		sample.HBMPct = float64(hbmUsed) / float64(hbmTotal) * 100
	}

	return sample
}

// nodeVizSeries returns the values of the per-node session series picked by
// sel, in chronological order. Samples sel rejects are skipped, so a gap in the
// data shows as a gap instead of as a zero.
func (m *Model) nodeVizSeries(sel func(NodeVizSample) (float64, bool)) []float64 {
	vals := make([]float64, 0, len(m.nodeVizHistory))
	for _, sample := range m.nodeVizHistory {
		if v, ok := sel(sample); ok {
			vals = append(vals, v)
		}
	}
	return vals
}

// nodeVizSeriesFor is the render-side entry: the session series recorded for
// exactly this node, or nil for a node the recorder has not seen. The ring is
// emptied by the recorder only on the next advancing snapshot, so a node
// entered before that would otherwise draw the previous node's trend — a shape
// neither node ever had — and indefinitely so while refreshes keep failing.
func (m *Model) nodeVizSeriesFor(node *model.NodeData, sel func(NodeVizSample) (float64, bool)) []float64 {
	if node == nil || m.nodeVizNodeName != node.Name {
		return nil
	}
	return m.nodeVizSeries(sel)
}

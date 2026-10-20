package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// The digit and Tab shortcuts must not switch the view while a mode that owns
// the keyboard is active: the switch would happen underneath the panel the user
// is looking at.
//
// TestFilterModeBlocksViewSwitchKeys pins filterMode and
// TestSearchModeConsumesViewSwitchKeys pins searchMode, but
// TestProbeViewSwitchKeysInOtherModes - the only test that exercises the other
// modes - has no assertions at all, it just t.Logf's what it saw. Removing the
// !viewSwitchBlocked() guard from the "2" case leaves that probe green, so the
// guards for logsMode, commandOutputMode, actionMenuMode and detailMode were
// unpinned. This probe asserts them.
//
// Each case also checks that the shortcut works again once the mode is left, so
// a guard that simply swallowed every digit cannot pass.
func TestViewSwitchGuardBlocksEveryOwningMode(t *testing.T) {
	modes := []struct {
		name  string
		set   func(m *Model)
		clear func(m *Model)
	}{
		{
			name:  "filterMode",
			set:   func(m *Model) { m.filterMode = true },
			clear: func(m *Model) { m.filterMode = false },
		},
		{
			name:  "logsMode",
			set:   func(m *Model) { m.logsMode = true },
			clear: func(m *Model) { m.logsMode = false },
		},
		{
			name:  "commandOutputMode",
			set:   func(m *Model) { m.commandOutputMode = true },
			clear: func(m *Model) { m.commandOutputMode = false },
		},
		{
			name:  "actionMenuMode",
			set:   func(m *Model) { m.actionMenuMode = true },
			clear: func(m *Model) { m.actionMenuMode = false },
		},
		{
			name:  "detailMode",
			set:   func(m *Model) { m.detailMode = true },
			clear: func(m *Model) { m.detailMode = false },
		},
	}

	// ViewOverview is the target of "1"; every other digit must be blocked too.
	start := ViewPods

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0"} {
				m := newTestModel(t)
				m.currentView = start
				m.detailMode = false
				// Make the optional views reachable, so 9/0 are only blocked by
				// the guard and not by their availability check.
				m.clusterData = &model.ClusterData{
					Queues:  []*model.QueueData{{Name: "q"}},
					Summary: &model.ClusterSummary{NPUCapacity: 8, SuperPodCount: 1},
				}
				mode.set(m)

				m.Update(keyMsg(key))
				if m.currentView != start {
					t.Errorf("digit %q switched view to %v while %s was active, want it left at %v",
						key, m.currentView, mode.name, start)
				}

				// Tab must be blocked as well.
				m2 := newTestModel(t)
				m2.currentView = start
				m2.detailMode = false
				m2.clusterData = m.clusterData
				mode.set(m2)
				m2.Update(tea.KeyMsg{Type: tea.KeyTab})
				if m2.currentView != start {
					t.Errorf("tab switched view to %v while %s was active, want it left at %v",
						m2.currentView, mode.name, start)
				}
			}

			// Non-vacuousness: with the mode cleared the shortcuts must work.
			m := newTestModel(t)
			m.currentView = start
			m.detailMode = false
			mode.clear(m)
			m.Update(keyMsg("1"))
			if m.currentView != ViewOverview {
				t.Fatalf("digit %q did not switch view after %s was cleared: currentView = %v",
					"1", mode.name, m.currentView)
			}
		})
	}
}

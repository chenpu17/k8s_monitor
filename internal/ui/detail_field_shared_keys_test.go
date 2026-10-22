package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The pod container info and the event basic info (both migrated in the same
// batch) render the byte-identical field labels "Reason" / "Message" through
// the shared detail.field.reason / detail.field.message keys. This test pins
// the sharing: editing either key's value in the bundle turns this test AND
// the golden tests of both files red at once, so the two views cannot drift.

func TestDetailFieldReasonMessageLockedAcrossPodAndEvent(t *testing.T) {
	forcedAsciiProfile(t)

	for locale, want := range map[string]struct{ reason, message string }{
		"en": {reason: "Reason: ", message: "Message:"},
		"zh": {reason: "原因: ", message: "消息:"},
	} {
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)

		// Pod container info: Reason and Message of a waiting container.
		pod := &model.PodData{
			Name: "pod-b", Namespace: "ns1", Phase: "Running",
			Containers: 1, ReadyContainers: 0,
			ContainerStates: []model.ContainerState{
				{Name: "c1", Image: "nginx:1.25", Ready: false, RestartCount: 4,
					State: "Waiting", Reason: "CrashLoopBackOff", Message: "boom"},
			},
		}
		podOut := m.renderPodContainerInfo(pod)
		if !strings.Contains(podOut, want.reason+"CrashLoopBackOff") {
			t.Errorf("[%s] pod container info must render the shared Reason label:\n%s", locale, podOut)
		}
		if !strings.Contains(podOut, want.message+" boom") {
			t.Errorf("[%s] pod container info must render the shared Message label:\n%s", locale, podOut)
		}

		// Event basic info: the same two labels for the event's fields.
		event := &model.EventData{
			Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container",
			Count: 7, InvolvedObject: "Pod/pod-a", InvolvedNamespace: "ns1",
			FirstTimestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			LastTimestamp:  time.Date(2026, 1, 2, 3, 10, 6, 0, time.UTC),
		}
		eventOut := m.renderEventBasicInfo(event)
		if !strings.Contains(eventOut, want.reason+"BackOff") {
			t.Errorf("[%s] event basic info must render the shared Reason label:\n%s", locale, eventOut)
		}
		if !strings.Contains(eventOut, want.message) {
			t.Errorf("[%s] event basic info must render the shared Message label:\n%s", locale, eventOut)
		}
	}
}

package datasource

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertNode(t *testing.T) {
	// Create a sample node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "",
			},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
				{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	// Convert
	nodeData := ConvertNode(node)

	// Verify
	if nodeData.Name != "test-node" {
		t.Errorf("Expected name 'test-node', got '%s'", nodeData.Name)
	}
	if nodeData.InternalIP != "10.0.0.1" {
		t.Errorf("Expected internal IP '10.0.0.1', got '%s'", nodeData.InternalIP)
	}
	if nodeData.ExternalIP != "1.2.3.4" {
		t.Errorf("Expected external IP '1.2.3.4', got '%s'", nodeData.ExternalIP)
	}
	if nodeData.Status != "Ready" {
		t.Errorf("Expected status 'Ready', got '%s'", nodeData.Status)
	}
	if len(nodeData.Roles) == 0 || nodeData.Roles[0] != "master" {
		t.Errorf("Expected role 'master', got %v", nodeData.Roles)
	}
}

func TestConvertPod(t *testing.T) {
	// Create a sample pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
			Containers: []corev1.Container{
				{Name: "test-container"},
			},
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodRunning,
			HostIP: "10.0.0.1",
			PodIP:  "10.244.0.1",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}

	// Convert
	podData := ConvertPod(pod)

	// Verify
	if podData.Name != "test-pod" {
		t.Errorf("Expected name 'test-pod', got '%s'", podData.Name)
	}
	if podData.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", podData.Namespace)
	}
	if podData.Node != "test-node" {
		t.Errorf("Expected node 'test-node', got '%s'", podData.Node)
	}
	if podData.Phase != "Running" {
		t.Errorf("Expected phase 'Running', got '%s'", podData.Phase)
	}
	if podData.ReadyContainers != 1 {
		t.Errorf("Expected 1 ready container, got %d", podData.ReadyContainers)
	}
}

func TestConvertEvent(t *testing.T) {
	// Create a sample event
	event := &corev1.Event{
		Type:    "Warning",
		Reason:  "FailedScheduling",
		Message: "0/1 nodes are available",
		Count:   5,
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "default",
		},
		Source: corev1.EventSource{
			Component: "scheduler",
		},
	}

	// Convert
	eventData := ConvertEvent(event)

	// Verify
	if eventData.Type != "Warning" {
		t.Errorf("Expected type 'Warning', got '%s'", eventData.Type)
	}
	if eventData.Reason != "FailedScheduling" {
		t.Errorf("Expected reason 'FailedScheduling', got '%s'", eventData.Reason)
	}
	if eventData.Count != 5 {
		t.Errorf("Expected count 5, got %d", eventData.Count)
	}
	if eventData.InvolvedObject != "Pod/test-pod" {
		t.Errorf("Expected involved object 'Pod/test-pod', got '%s'", eventData.InvolvedObject)
	}
	if eventData.Source != "scheduler" {
		t.Errorf("Expected source 'scheduler', got '%s'", eventData.Source)
	}
}

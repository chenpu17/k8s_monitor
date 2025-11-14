package diagnostic

import (
	"context"
	"fmt"
	"strings"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// KubeletAccessStatus records whether the current identity can reach kubelet summary endpoints.
type KubeletAccessStatus struct {
	ProxyAllowed bool
	ProxyMessage string
	CheckedAt    time.Time
}

// Message returns a human-friendly summary for display in the UI.
func (s *KubeletAccessStatus) Message() string {
	if s == nil || s.ProxyAllowed {
		return ""
	}

	message := s.ProxyMessage
	if message == "" {
		message = "当前凭证无法访问 kubelet proxy (需要 get nodes/proxy 权限)"
	}

	// Encourage the operator to validate via kubectl.
	return fmt.Sprintf("%s • 请执行 `kubectl auth can-i get nodes/proxy` 或授予相应 RBAC", message)
}

// CheckKubeletAccess performs a SelfSubjectAccessReview for nodes/proxy.
func CheckKubeletAccess(ctx context.Context, client authorizationclient.AuthorizationV1Interface) (*KubeletAccessStatus, error) {
	sar := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:        "get",
				Resource:    "nodes",
				Subresource: "proxy",
				Group:       "",
			},
		},
	}

	resp, err := client.SelfSubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	status := &KubeletAccessStatus{
		ProxyAllowed: resp.Status.Allowed,
		CheckedAt:    time.Now(),
	}

	if resp.Status.Allowed {
		return status, nil
	}

	var details []string
	if resp.Status.Reason != "" {
		details = append(details, resp.Status.Reason)
	}
	if resp.Status.EvaluationError != "" {
		details = append(details, resp.Status.EvaluationError)
	}

	status.ProxyMessage = strings.TrimSpace(strings.Join(details, " • "))

	return status, nil
}

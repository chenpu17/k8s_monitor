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
	// MessageKey is the i18n key for the denial message (diagnostic.*), the
	// same convention as model.Alert.MessageKey: ProxyMessage keeps the
	// original text for export/log paths, the UI renders MessageKey through
	// the localizer when it is set and falls back to ProxyMessage otherwise.
	// Raw API reasons carried in ProxyMessage are data passthrough and have
	// no key.
	MessageKey string `json:"message_key,omitempty"`
	// MessageArgs holds the named template arguments for MessageKey.
	MessageArgs map[string]interface{} `json:"message_args,omitempty"`
	CheckedAt   time.Time
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

// Message keys for the translatable denial messages. The zh bundle values are
// byte-identical to the historical Message() output (locked by golden tests);
// the en values give English locales English copy where Message() used to leak
// the hardcoded Chinese text.
const (
	keyAccessReviewFailed = "diagnostic.access_review_failed"
	keyProxyDeniedDetails = "diagnostic.proxy_denied_details"
	keyProxyDeniedDefault = "diagnostic.proxy_denied_default"
)

// AccessReviewFailedStatus builds the cached denial for a SelfSubjectAccessReview
// that could not even be created (e.g. the identity lacks create permission on
// selfsubjectaccessreviews). ProxyMessage keeps the historical text; the key
// lets the UI render it through the localizer.
func AccessReviewFailedStatus(err error) *KubeletAccessStatus {
	return &KubeletAccessStatus{
		ProxyAllowed: false,
		ProxyMessage: fmt.Sprintf("Access review failed: %v", err),
		MessageKey:   keyAccessReviewFailed,
		MessageArgs:  map[string]interface{}{"Error": err.Error()},
		CheckedAt:    time.Now(),
	}
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

	// Denial: the joined details (if any) are the message, and the key lets
	// the UI render it through the localizer. Without details Message() falls
	// back to its default text, keyed as proxy_denied_default.
	var details []string
	if resp.Status.Reason != "" {
		details = append(details, resp.Status.Reason)
	}
	if resp.Status.EvaluationError != "" {
		details = append(details, resp.Status.EvaluationError)
	}

	status.ProxyMessage = strings.TrimSpace(strings.Join(details, " • "))
	if status.ProxyMessage == "" {
		status.MessageKey = keyProxyDeniedDefault
	} else {
		status.MessageKey = keyProxyDeniedDetails
		status.MessageArgs = map[string]interface{}{"Reason": status.ProxyMessage}
	}

	return status, nil
}

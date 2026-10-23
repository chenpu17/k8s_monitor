package diagnostic

// The kubelet access denial messages are the last cross-package hardcoded UI
// copy: Message() carries the historical (zh) text for export/log paths, and
// the MessageKey/MessageArgs pair lets the overview render the same message
// through the localizer. The zh bundle values must stay byte-identical to the
// Message() output (golden strings below are the anchor), while en must render
// English copy instead of leaking the Chinese default.

import (
	"context"
	"errors"
	"strings"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"github.com/chenpu/k8s-monitor/internal/i18n"
)

// The historical Message() output for each denial shape, frozen as the zh
// golden. Any byte drift in the bundle breaks these.
const (
	sarErrMsg = "selfsubjectaccessreviews.authorization.k8s.io is forbidden"

	zhSuffix = " • 请执行 `kubectl auth can-i get nodes/proxy` 或授予相应 RBAC"

	zhAccessReviewFailed = "Access review failed: " + sarErrMsg + zhSuffix
	zhProxyDeniedDetails = "forbidden: no proxy" + zhSuffix
	zhProxyDeniedDefault = "当前凭证无法访问 kubelet proxy (需要 get nodes/proxy 权限)" + zhSuffix
)

// fakeSARInterface implements only Create; embedding the interface keeps every
// other method a nil-call panic, which fails loudly if the code path grows.
type fakeSARInterface struct {
	authorizationclient.SelfSubjectAccessReviewInterface
	resp *authorizationv1.SelfSubjectAccessReview
	err  error
}

func (f *fakeSARInterface) Create(_ context.Context, _ *authorizationv1.SelfSubjectAccessReview, _ metav1.CreateOptions) (*authorizationv1.SelfSubjectAccessReview, error) {
	return f.resp, f.err
}

type fakeAuthzClient struct {
	authorizationclient.AuthorizationV1Interface
	sar authorizationclient.SelfSubjectAccessReviewInterface
}

func (f *fakeAuthzClient) SelfSubjectAccessReviews() authorizationclient.SelfSubjectAccessReviewInterface {
	return f.sar
}

func deniedResponse(reason, evaluationError string) *authorizationv1.SelfSubjectAccessReview {
	return &authorizationv1.SelfSubjectAccessReview{
		Status: authorizationv1.SubjectAccessReviewStatus{
			Allowed:         false,
			Reason:          reason,
			EvaluationError: evaluationError,
		},
	}
}

// TestCheckKubeletAccessMessageKeys locks the key/args each construction site
// sets and the byte-identity between Message() and the zh render.
func TestCheckKubeletAccessMessageKeys(t *testing.T) {
	en := i18n.NewLocalizer("en")
	zh := i18n.NewLocalizer("zh")

	sarErrMsg := "selfsubjectaccessreviews.authorization.k8s.io is forbidden"

	cases := []struct {
		name         string
		client       authorizationclient.AuthorizationV1Interface
		build        func() *KubeletAccessStatus
		wantKey      string
		wantArgs     map[string]interface{}
		wantMessage  string // Message() output, and the zh TF render byte for byte
		wantEnSubstr string // English copy the en render must contain
	}{
		{
			name: "access review itself failed",
			build: func() *KubeletAccessStatus {
				return AccessReviewFailedStatus(errors.New(sarErrMsg))
			},
			wantKey:      "diagnostic.access_review_failed",
			wantArgs:     map[string]interface{}{"Error": sarErrMsg},
			wantMessage:  zhAccessReviewFailed,
			wantEnSubstr: "Access review failed: " + sarErrMsg,
		},
		{
			name: "denied with reason details",
			client: &fakeAuthzClient{sar: &fakeSARInterface{
				resp: deniedResponse("forbidden: no proxy", ""),
			}},
			wantKey:      "diagnostic.proxy_denied_details",
			wantArgs:     map[string]interface{}{"Reason": "forbidden: no proxy"},
			wantMessage:  zhProxyDeniedDetails,
			wantEnSubstr: "forbidden: no proxy",
		},
		{
			name: "denied with reason and evaluation error",
			client: &fakeAuthzClient{sar: &fakeSARInterface{
				resp: deniedResponse("forbidden: no proxy", "evaluator failed"),
			}},
			wantKey:      "diagnostic.proxy_denied_details",
			wantArgs:     map[string]interface{}{"Reason": "forbidden: no proxy • evaluator failed"},
			wantMessage:  "forbidden: no proxy • evaluator failed" + zhSuffix,
			wantEnSubstr: "evaluator failed",
		},
		{
			name: "denied without details falls back to default",
			client: &fakeAuthzClient{sar: &fakeSARInterface{
				resp: deniedResponse("", ""),
			}},
			wantKey:      "diagnostic.proxy_denied_default",
			wantArgs:     nil,
			wantMessage:  zhProxyDeniedDefault,
			wantEnSubstr: "Current identity cannot access the kubelet proxy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var status *KubeletAccessStatus
			if tc.client != nil {
				var err error
				status, err = CheckKubeletAccess(context.Background(), tc.client)
				if err != nil {
					t.Fatalf("CheckKubeletAccess returned error: %v", err)
				}
			} else {
				status = tc.build()
			}
			if status.ProxyAllowed {
				t.Fatalf("status unexpectedly allowed")
			}
			if status.MessageKey != tc.wantKey {
				t.Errorf("MessageKey = %q, want %q", status.MessageKey, tc.wantKey)
			}
			if len(status.MessageArgs) != len(tc.wantArgs) {
				t.Errorf("MessageArgs = %v, want %v", status.MessageArgs, tc.wantArgs)
			}
			for k, v := range tc.wantArgs {
				if status.MessageArgs[k] != v {
					t.Errorf("MessageArgs[%q] = %v, want %v", k, status.MessageArgs[k], v)
				}
			}

			// The raw path must be untouched, and the zh render must be
			// byte-identical to it.
			if got := status.Message(); got != tc.wantMessage {
				t.Errorf("Message() = %q, want %q", got, tc.wantMessage)
			}
			if got := zh.TF(status.MessageKey, status.MessageArgs); got != status.Message() {
				t.Errorf("zh TF(%q, %v) = %q, Message() = %q (not byte-identical)",
					status.MessageKey, status.MessageArgs, got, status.Message())
			}

			// en must render English copy, not the historical Chinese.
			enRendered := en.TF(status.MessageKey, status.MessageArgs)
			if !strings.Contains(enRendered, tc.wantEnSubstr) {
				t.Errorf("en render %q missing %q", enRendered, tc.wantEnSubstr)
			}
			for _, bad := range []string{"请执行", "当前凭证", "<no value>", "%!"} {
				if strings.Contains(enRendered, bad) {
					t.Errorf("en render %q contains %q", enRendered, bad)
				}
			}
		})
	}
}

// TestAllowedStatusHasNoKey: an allowed check carries no message key and an
// empty Message, so the UI keeps falling back to raw data.
func TestAllowedStatusHasNoKey(t *testing.T) {
	client := &fakeAuthzClient{sar: &fakeSARInterface{
		resp: &authorizationv1.SelfSubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
		},
	}}
	status, err := CheckKubeletAccess(context.Background(), client)
	if err != nil {
		t.Fatalf("CheckKubeletAccess returned error: %v", err)
	}
	if !status.ProxyAllowed {
		t.Fatalf("status not allowed")
	}
	if status.Message() != "" {
		t.Errorf("Message() = %q, want empty", status.Message())
	}
	if status.MessageKey != "" || status.MessageArgs != nil {
		t.Errorf("allowed status carries key/args: %q %v", status.MessageKey, status.MessageArgs)
	}
}

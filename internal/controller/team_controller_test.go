package controller

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	forailv1 "github.com/forail-platform/forail-operator/api/v1alpha1"
)

func TestTeamLifecycle(t *testing.T) {
	mock := newMockForail()
	srv, _ := mock.start(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stop := newManager(t, ctx, srv.URL, "test-token")
	defer stop()

	cr := &forailv1.Team{
		ObjectMeta: metav1.ObjectMeta{Name: "oncall", Namespace: "default"},
		Spec: forailv1.TeamSpec{
			Description:  "On-call rotation",
			Organization: "Default",
			Users:        []string{"admin"},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("create CR: %v", err)
	}

	if !pollUntil(t, 10*time.Second, func() bool {
		var got forailv1.Team
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "oncall", Namespace: "default"}, &got); err != nil {
			return false
		}
		return got.Status.ForailID > 0
	}) {
		t.Fatal("timeout: forailId not set")
	}
	if mock.CallCount("POST teams") < 1 {
		t.Fatal("expected POST teams")
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		return mock.CallCount("ASSOCIATE team user") >= 1
	}) {
		t.Fatal("timeout: ASSOCIATE team user never called")
	}

	if err := k8sClient.Delete(ctx, cr); err != nil {
		t.Fatalf("delete CR: %v", err)
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		return mock.CallCount("DELETE team") >= 1
	}) {
		t.Fatal("timeout: DELETE team never called")
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		var x forailv1.Team
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "oncall", Namespace: "default"}, &x)
		return apierrors.IsNotFound(err)
	}) {
		t.Fatal("timeout: CR not removed after finalizer")
	}
}

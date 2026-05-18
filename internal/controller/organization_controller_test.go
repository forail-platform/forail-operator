package controller

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	forgev1 "github.com/forgeplatform/forge-operator/api/v1alpha1"
)

func TestOrganizationLifecycle(t *testing.T) {
	mock := newMockForge()
	srv, _ := mock.start(t)

	// Wipe the seed Default org so create-by-name path runs cleanly.
	mock.mu.Lock()
	delete(mock.organizations, 1)
	mock.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stop := newManager(t, ctx, srv.URL, "test-token")
	defer stop()

	cr := &forgev1.Organization{
		ObjectMeta: metav1.ObjectMeta{Name: "platform-team", Namespace: "default"},
		Spec: forgev1.OrganizationSpec{
			Description: "platform tier",
			MaxHosts:    250,
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("create CR: %v", err)
	}

	if !pollUntil(t, 10*time.Second, func() bool {
		var got forgev1.Organization
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "platform-team", Namespace: "default"}, &got); err != nil {
			return false
		}
		return got.Status.ForgeID > 0
	}) {
		t.Fatal("timeout: forgeId not set")
	}
	if mock.CallCount("POST organizations") < 1 {
		t.Fatalf("expected POST organizations, got %d", mock.CallCount("POST organizations"))
	}

	// Drift: change MaxHosts -> PATCH expected.
	var got forgev1.Organization
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: "platform-team", Namespace: "default"}, &got)
	got.Spec.MaxHosts = 500
	if err := k8sClient.Update(ctx, &got); err != nil {
		t.Fatalf("update CR: %v", err)
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		return mock.CallCount("PATCH organization") >= 1
	}) {
		t.Fatal("timeout: PATCH organization never called")
	}

	if err := k8sClient.Delete(ctx, &got); err != nil {
		t.Fatalf("delete CR: %v", err)
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		return mock.CallCount("DELETE organization") >= 1
	}) {
		t.Fatal("timeout: DELETE organization never called")
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		var x forgev1.Organization
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "platform-team", Namespace: "default"}, &x)
		return apierrors.IsNotFound(err)
	}) {
		t.Fatal("timeout: CR not removed after finalizer")
	}
}

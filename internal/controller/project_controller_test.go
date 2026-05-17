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

func TestProjectLifecycle(t *testing.T) {
	mock := newMockForge()
	srv, _ := mock.start(t)

	// The mock seeds Demo Project at id=1 — clear it so we exercise the
	// create path cleanly. Organization "Default" stays in.
	mock.mu.Lock()
	delete(mock.projects, 1)
	mock.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stop := newManager(t, ctx, srv.URL, "test-token")
	defer stop()

	cr := &forgev1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "myproject", Namespace: "default"},
		Spec: forgev1.ProjectSpec{
			Description:  "Internal automation",
			Organization: "Default",
			ScmType:      "git",
			ScmURL:       "https://github.com/mycorp/automation.git",
			ScmBranch:    "main",
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("create CR: %v", err)
	}

	if !pollUntil(t, 10*time.Second, func() bool {
		var got forgev1.Project
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "myproject", Namespace: "default"}, &got); err != nil {
			return false
		}
		return got.Status.ForgeID > 0
	}) {
		t.Fatal("timeout: forgeId not set")
	}
	if mock.CallCount("POST projects") < 1 {
		t.Fatalf("expected POST projects, got %d", mock.CallCount("POST projects"))
	}

	// Drift: change branch -> PATCH expected.
	var got forgev1.Project
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: "myproject", Namespace: "default"}, &got)
	got.Spec.ScmBranch = "develop"
	if err := k8sClient.Update(ctx, &got); err != nil {
		t.Fatalf("update CR: %v", err)
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		return mock.CallCount("PATCH project") >= 1
	}) {
		t.Fatal("timeout: PATCH project never called")
	}

	// Delete: finalizer should hit Forge.
	if err := k8sClient.Delete(ctx, &got); err != nil {
		t.Fatalf("delete CR: %v", err)
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		return mock.CallCount("DELETE project") >= 1
	}) {
		t.Fatal("timeout: DELETE project never called")
	}
	if !pollUntil(t, 10*time.Second, func() bool {
		var x forgev1.Project
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "myproject", Namespace: "default"}, &x)
		return apierrors.IsNotFound(err)
	}) {
		t.Fatal("timeout: CR not removed after finalizer")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.projects) != 0 {
		t.Fatalf("expected 0 projects in mock, got %d", len(mock.projects))
	}
}

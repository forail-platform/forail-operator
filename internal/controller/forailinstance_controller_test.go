package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	forailv1 "github.com/forail-platform/forail-operator/api/v1alpha1"
)

func TestForailInstanceProbe(t *testing.T) {
	mock := newMockForail()
	srv, _ := mock.start(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stop := newManager(t, ctx, srv.URL, "test-token")
	defer stop()

	tokSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "forail-eu-token", Namespace: "default"},
		StringData: map[string]string{"token": "test-token"},
	}
	if err := k8sClient.Create(ctx, tokSecret); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	cr := &forailv1.ForailInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "forail-eu", Namespace: "default"},
		Spec: forailv1.ForailInstanceSpec{
			URL:                srv.URL,
			InsecureSkipVerify: true,
			TokenSecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "forail-eu-token"},
				Key:                  "token",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("create CR: %v", err)
	}

	if !pollUntil(t, 10*time.Second, func() bool {
		var got forailv1.ForailInstance
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "forail-eu", Namespace: "default"}, &got); err != nil {
			return false
		}
		return got.Status.Reachable && got.Status.ServerVersion != ""
	}) {
		t.Fatal("timeout: ForailInstance status never set reachable")
	}
	if mock.CallCount("GET ping") < 1 {
		t.Fatal("expected GET ping")
	}
}

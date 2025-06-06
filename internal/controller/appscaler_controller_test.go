package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppScaler Controller", func() {
	var (
		ctx        = context.Background()
		deployment *appsv1.Deployment
		mockServer *httptest.Server
	)

	BeforeEach(func() {
		// Set up mock JetStream HTTP server
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"streams": []map[string]interface{}{
					{
						"config": map[string]interface{}{
							"name": "test-stream",
						},
						"state": map[string]interface{}{
							"messages": 200, // simulate high queue length
						},
					},
				},
			})
		}))

		// Create the test Deployment to scale
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointerToInt32(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "test", Image: "busybox"},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
	})

	AfterEach(func() {
		mockServer.Close()
		_ = k8sClient.Delete(ctx, deployment)
	})

	It("should scale up the deployment when queue is large", func() {
		scaler := &appv1.AppScaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-scaler",
				Namespace: "default",
			},
			Spec: appv1.AppScalerSpec{
				DeploymentName:      "test-deploy",
				MinReplicas:         1,
				MaxReplicas:         5,
				NatsMonitoringURL:   mockServer.URL,
				Subject:             "test-stream",
				ScaleUpThreshold:    100,
				ScaleDownThreshold:  50,
				PollIntervalSeconds: 2,
			},
		}
		Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

		Eventually(func() int32 {
			var dep appsv1.Deployment
			_ = k8sClient.Get(ctx, client.ObjectKey{Name: "test-deploy", Namespace: "default"}, &dep)
			return *dep.Spec.Replicas
		}, 10*time.Second, 1*time.Second).Should(BeNumerically(">", 1))
	})
})

func pointerToInt32(i int32) *int32 {
	return &i
}

package controller

import (
	"context"
	"encoding/json"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	autoscalev1 "github.com/joshuazeltser/nats-scale-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	scheme1 = runtime.NewScheme()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	// Register schemes
	utilruntime.Must(clientgoscheme.AddToScheme(scheme1))
	utilruntime.Must(autoscalev1.AddToScheme(scheme1)) // your CRD

	// Start envtest environment
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../config/crd/bases", // path to your CRD YAMLs
		},
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme1})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("AppScaler Controller", func() {
	var (
		deployment *appsv1.Deployment
		mockServer *httptest.Server
	)

	BeforeEach(func() {
		// Setup mock JetStream HTTP server
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

		// Create Deployment to scale
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
		scaler := &autoscalev1.AppScaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-scaler",
				Namespace: "default",
			},
			Spec: autoscalev1.AppScalerSpec{
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
		}, 15*time.Second, 1*time.Second).Should(BeNumerically(">", 1))
	})
})

func pointerToInt32(i int32) *int32 {
	return &i
}

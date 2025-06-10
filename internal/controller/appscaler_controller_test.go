package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	autoscalev1 "github.com/joshuazeltser/nats-scale-operator/api/v1"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = autoscalev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("AppScaler Controller", func() {
	Context("When reconciling an AppScaler resource", func() {
		const (
			AppScalerName      = "test-appscaler"
			AppScalerNamespace = "default"
			DeploymentName     = "test-deployment"
			timeout            = time.Second * 10
			duration           = time.Second * 10
			interval           = time.Millisecond * 250
		)

		var (
			mockServer       *httptest.Server
			appScaler        *autoscalev1.AppScaler
			deployment       *appsv1.Deployment
			reconciler       *AppScalerReconciler
			mockMessageCount int
		)

		BeforeEach(func() {
			// Setup mock NATS server
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := fmt.Sprintf(`{
					"streams": [{
						"name": "test-stream",
						"state": {
							"messages": %d
						}
					}]
				}`, mockMessageCount)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)

				if err, _ := w.Write([]byte(response)); err != nil {
					return
				}
			}))

			// Create test deployment
			replicas := int32(2)
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      DeploymentName,
					Namespace: AppScalerNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "test",
								Image: "nginx:latest",
							}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())

			// Create AppScaler resource
			appScaler = &autoscalev1.AppScaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppScalerName,
					Namespace: AppScalerNamespace,
				},
				Spec: autoscalev1.AppScalerSpec{
					DeploymentName:      DeploymentName,
					NatsMonitoringURL:   mockServer.URL,
					Stream:              "test-stream",
					MinReplicas:         1,
					MaxReplicas:         5,
					ScaleUpThreshold:    10,
					ScaleDownThreshold:  2,
					PollIntervalSeconds: 1,
				},
			}
			Expect(k8sClient.Create(ctx, appScaler)).Should(Succeed())

			// Setup reconciler
			reconciler = &AppScalerReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}
		})

		AfterEach(func() {
			// Cleanup resources
			Expect(k8sClient.Delete(ctx, appScaler)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, deployment)).Should(Succeed())
			mockServer.Close()
		})

		It("Should scale up deployment when message count exceeds threshold", func() {
			By("Setting high message count to trigger scale up")
			mockMessageCount = 15 // Above ScaleUpThreshold of 10

			By("Triggering reconciliation")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      AppScalerName,
					Namespace: AppScalerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying deployment was scaled up")
			Eventually(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      DeploymentName,
					Namespace: AppScalerNamespace,
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3))) // Should increase from 2 to 3

			By("Verifying it doesn't scale beyond max replicas")
			// Set message count high and trigger multiple reconciliations
			mockMessageCount = 50

			// Scale to max replicas
			for i := 0; i < 5; i++ {
				_, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      AppScalerName,
						Namespace: AppScalerNamespace,
					},
				})
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(100 * time.Millisecond)
			}

			Eventually(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      DeploymentName,
					Namespace: AppScalerNamespace,
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(5))) // Should not exceed MaxReplicas of 5
		})

		It("Should scale down deployment when message count is below threshold", func() {
			By("Starting with a higher replica count")
			// First scale up the deployment to 4 replicas
			deployment.Spec.Replicas = func() *int32 { i := int32(4); return &i }()
			Expect(k8sClient.Update(ctx, deployment)).Should(Succeed())

			By("Setting low message count to trigger scale down")
			mockMessageCount = 1 // Below ScaleDownThreshold of 2

			By("Triggering reconciliation")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      AppScalerName,
					Namespace: AppScalerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying deployment was scaled down")
			Eventually(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      DeploymentName,
					Namespace: AppScalerNamespace,
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3))) // Should decrease from 4 to 3

			By("Verifying it doesn't scale below min replicas")
			// Continue scaling down to test min replicas limit
			for i := 0; i < 5; i++ {
				_, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      AppScalerName,
						Namespace: AppScalerNamespace,
					},
				})
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(100 * time.Millisecond)
			}

			Eventually(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      DeploymentName,
					Namespace: AppScalerNamespace,
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1))) // Should not go below MinReplicas of 1
		})

		It("Should not scale when message count is between thresholds", func() {
			By("Setting message count between thresholds")
			mockMessageCount = 5 // Between ScaleDownThreshold (2) and ScaleUpThreshold (10)

			By("Getting initial replica count")
			var initialDeployment appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      DeploymentName,
				Namespace: AppScalerNamespace,
			}, &initialDeployment)).Should(Succeed())
			initialReplicas := *initialDeployment.Spec.Replicas

			By("Triggering reconciliation")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      AppScalerName,
					Namespace: AppScalerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying deployment replicas remain unchanged")
			Consistently(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      DeploymentName,
					Namespace: AppScalerNamespace,
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, duration, interval).Should(Equal(initialReplicas))
		})

		It("Should handle NATS server errors gracefully", func() {
			By("Stopping the mock server to simulate NATS unavailability")
			mockServer.Close()

			By("Triggering reconciliation")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      AppScalerName,
					Namespace: AppScalerNamespace,
				},
			})

			By("Verifying error is returned")
			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			By("Verifying deployment remains unchanged")
			var updatedDeployment appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      DeploymentName,
				Namespace: AppScalerNamespace,
			}, &updatedDeployment)).Should(Succeed())
			Expect(*updatedDeployment.Spec.Replicas).To(Equal(int32(2))) // Should remain at initial value
		})
	})
})

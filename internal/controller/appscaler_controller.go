package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalev1 "github.com/joshuazeltser/nats-scale-operator/api/v1"
)

// AppScalerReconciler reconciles a AppScaler object
type AppScalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type ScalingEvent struct {
	Timestamp      time.Time `json:"timestamp"`
	DeploymentName string    `json:"deployment"`
	OldReplicas    int32     `json:"oldReplicas"`
	NewReplicas    int32     `json:"newReplicas"`
	Reason         string    `json:"reason"`
}

var scalingHistory []ScalingEvent
var scalingHistoryLock sync.Mutex

// Add these variables to ensure HTTP server starts only once
var httpServerOnce sync.Once

// RBAC permissions for AppScaler custom resource
// +kubebuilder:rbac:groups=autoscale.example.com,resources=appscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscale.example.com,resources=appscalers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscale.example.com,resources=appscalers/finalizers,verbs=update

// RBAC permissions for Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update

// RBAC permissions for events (optional but recommended for debugging)
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AppScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Start HTTP server once
	httpServerOnce.Do(func() {
		go r.startHTTPServer()
	})

	var scaler autoscalev1.AppScaler
	if err := r.Get(ctx, req.NamespacedName, &scaler); err != nil {
		if errors.IsNotFound(err) {
			log.Info("AppScaler resource not found")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AppScaler")
		return ctrl.Result{}, err
	}

	var deploy appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{
		Name:      scaler.Spec.DeploymentName,
		Namespace: scaler.Namespace,
	}, &deploy); err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	queueLen, err := getPendingMessagesFromJetStream(scaler.Spec.NatsMonitoringURL, scaler.Spec.Stream)
	if err != nil {
		log.Error(err, "Failed to get queue length from NATS")
		return ctrl.Result{}, err
	}

	log.Info("NATS queue length", "subject", scaler.Spec.Stream, "messages", queueLen)

	currentReplicas := *deploy.Spec.Replicas
	desiredReplicas := currentReplicas

	if queueLen >= scaler.Spec.ScaleUpThreshold && currentReplicas < scaler.Spec.MaxReplicas {
		desiredReplicas++
	} else if queueLen <= scaler.Spec.ScaleDownThreshold && currentReplicas > scaler.Spec.MinReplicas {
		desiredReplicas--
	}

	if desiredReplicas != currentReplicas {
		log.Info("Scaling deployment", "from", currentReplicas, "to", desiredReplicas)
		deploy.Spec.Replicas = &desiredReplicas
		if err := r.Update(ctx, &deploy); err != nil {
			log.Error(err, "Failed to update deployment replicas")
			return ctrl.Result{}, err
		}

		// Save scaling event
		scalingHistoryLock.Lock()
		scalingHistory = append(scalingHistory, ScalingEvent{
			Timestamp:      time.Now(),
			DeploymentName: deploy.Name,
			OldReplicas:    currentReplicas,
			NewReplicas:    desiredReplicas,
			Reason:         fmt.Sprintf("Queue length: %d", queueLen),
		})
		// Keep last 100 entries
		if len(scalingHistory) > 100 {
			scalingHistory = scalingHistory[len(scalingHistory)-100:]
		}
		scalingHistoryLock.Unlock()
	}

	return ctrl.Result{
		RequeueAfter: time.Duration(scaler.Spec.PollIntervalSeconds) * time.Second,
	}, nil
}

// startHTTPServer starts a simple HTTP server for scaling history
func (r *AppScalerReconciler) startHTTPServer() {
	http.HandleFunc("/scaling-history", r.handleScalingHistory)

	log := logf.Log.WithName("http-server")
	log.Info("Starting HTTP server for scaling history", "port", 8080)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Error(err, "HTTP server failed")
	}
}

// handleScalingHistory returns recent scaling events
func (r *AppScalerReconciler) handleScalingHistory(w http.ResponseWriter, req *http.Request) {
	scalingHistoryLock.Lock()
	defer scalingHistoryLock.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(scalingHistory); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalev1.AppScaler{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Named("appscaler").
		Complete(r)
}

func getPendingMessagesFromJetStream(natsURL, stream string) (int, error) {
	url := fmt.Sprintf("%s/jsz?streams=1&stream=%s", natsURL, stream)
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to query JetStream: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't return it as it's in a defer
			logf.Log.Error(closeErr, "Failed to close response body")
		}
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read JetStream response: %w", err)
	}

	var jsData struct {
		AccountDetails []struct {
			StreamDetail []struct {
				Name  string `json:"name"`
				State struct {
					Messages json.Number `json:"messages"`
				} `json:"state"`
			} `json:"stream_detail"`
		} `json:"account_details"`
	}

	if err := json.Unmarshal(body, &jsData); err != nil {
		return 0, fmt.Errorf("failed to parse JetStream response: %w", err)
	}

	for _, account := range jsData.AccountDetails {
		for _, streamDetail := range account.StreamDetail {
			if streamDetail.Name == stream {
				messages, err := streamDetail.State.Messages.Int64()
				if err != nil {
					return 0, fmt.Errorf("failed to convert messages to int: %w", err)
				}
				return int(messages), nil
			}
		}
	}

	return 0, fmt.Errorf("stream %s not found", stream)
}

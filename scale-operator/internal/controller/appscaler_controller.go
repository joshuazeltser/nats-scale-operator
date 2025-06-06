/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalev1 "github.com/joshuazeltser/nats-scale-operator.git/api/v1"
)

// AppScalerReconciler reconciles a AppScaler object
type AppScalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscale.example.com,resources=appscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscale.example.com,resources=appscalers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscale.example.com,resources=appscalers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AppScaler object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *AppScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

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

	queueLen, err := getPendingMessagesFromJetStream(scaler.Spec.NatsMonitoringURL, scaler.Spec.Stream, scaler.Spec.Consumer)
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
	}

	return ctrl.Result{
		RequeueAfter: time.Duration(scaler.Spec.PollIntervalSeconds) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalev1.AppScaler{}).
		Named("appscaler").
		Complete(r)
}

func getPendingMessagesFromJetStream(natsURL, stream, consumer string) (int, error) {
	url := fmt.Sprintf("%s/jsz?consumers=true", natsURL)
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to query JetStream: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read JetStream response: %w", err)
	}

	var jsData struct {
		Consumers []struct {
			Name       string `json:"name"`
			Stream     string `json:"stream_name"`
			NumPending int    `json:"num_pending"`
		} `json:"consumers"`
	}

	if err := json.Unmarshal(body, &jsData); err != nil {
		return 0, fmt.Errorf("failed to parse JetStream response: %w", err)
	}

	for _, c := range jsData.Consumers {
		if c.Stream == stream && c.Name == consumer {
			return c.NumPending, nil
		}
	}

	return 0, fmt.Errorf("consumer %s on stream %s not found", consumer, stream)
}

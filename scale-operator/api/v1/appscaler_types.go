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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AppScalerSpec defines the desired state of AppScaler.
type AppScalerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	DeploymentName      string `json:"deploymentName"`
	Namespace           string `json:"namespace"`
	MinReplicas         int32  `json:"minReplicas"`
	MaxReplicas         int32  `json:"maxReplicas"`
	NatsMonitoringURL   string `json:"natsMonitoringUrl"`
	Subject             string `json:"subject"`
	ScaleUpThreshold    int    `json:"scaleUpThreshold"`
	ScaleDownThreshold  int    `json:"scaleDownThreshold"`
	PollIntervalSeconds int32  `json:"pollIntervalSeconds"`
	// For JetStream support
	Stream   string `json:"stream"`
	Consumer string `json:"consumer"`
}

// AppScalerStatus defines the observed state of AppScaler.
type AppScalerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	CurrentReplicas int32 `json:"currentReplicas"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AppScaler is the Schema for the appscalers API.
type AppScaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppScalerSpec   `json:"spec,omitempty"`
	Status AppScalerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppScalerList contains a list of AppScaler.
type AppScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppScaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppScaler{}, &AppScalerList{})
}

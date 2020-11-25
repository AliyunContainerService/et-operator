/*

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

package v1alpha1

import (
	"fmt"
	common "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScaleInSpec defines the desired state of ScaleIn
type ScaleInSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ScaleScriptSpec `json:",inline"`

	// Optional number of retries to execute script.
	// +optional
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`

	ToDelete *ToDeleteSpec `json:"toDelete,omitempty"`

	Selector Selector `json:"selector,omitempty"`
}

type ToDeleteSpec struct {
	Count    int      `json:"count,omitempty"`
	PodNames []string `json:"podNames,omitempty"`
}

// ScaleInStatus defines the observed state of ScaleIn
type ScaleInStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	common.JobStatus `json:",inline"`

	// record delete pods for scalein
	ToDeletePods []string `json:"toDeletePods,omitempty"`
}

// +kubebuilder:subresource:Spec
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.conditions[-1:].type`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:object:root=true

// ScaleIn is the Schema for the scaleins API
type ScaleIn struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ScaleInSpec `json:"spec,omitempty"`
	// Most recently observed status of the PyTorchJob.
	// Read-only (modified by the system).
	Status ScaleInStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScaleInList contains a list of ScaleIn
type ScaleInList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScaleIn `json:"items"`
}

const (
	SCALER_TYPE_SCALEIN  = "ScalingIn"
	SCALER_TYPE_SCALEOUT = "ScalingOut"
)

func (s *ScaleIn) GetJobStatus() *common.JobStatus {
	return &s.Status.JobStatus
}

func (s *ScaleIn) GetStatus() interface{} {
	return &s.Status
}

func (s *ScaleIn) GetFullName() string {
	return fmt.Sprintf("ScaleIn:%s/%s", s.Namespace, s.Name)
}

func (s *ScaleIn) GetScaleType() string {
	return SCALER_TYPE_SCALEIN
}

func (s *ScaleIn) GetPodNames() []string {
	return s.Status.ToDeletePods
}

func (s *ScaleIn) GetScriptSpec() ScaleScriptSpec {
	return s.Spec.ScaleScriptSpec
}

func (s *ScaleIn) GetSelector() Selector {
	return s.Spec.Selector
}

func init() {
	SchemeBuilder.Register(&ScaleIn{}, &ScaleInList{})
}

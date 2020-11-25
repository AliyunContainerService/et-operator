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

type EnvSpec struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// ScaleOutSpec defines the desired state of ScaleOut
type ScaleOutSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ScaleScriptSpec `json:",inline"`

	// Optional number of retries to execute script.
	// +optional
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`

	ToAdd *ToAddSpec `json:"toAdd,omitempty"`

	Selector Selector `json:"selector,omitempty"`
}

type ScaleScriptSpec struct {
	Script string `json:"script,omitempty"`
	// Optional number of timeout to execute script.
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`

	Env []EnvSpec `json:"env,omitempty"`
}

func (s ScaleScriptSpec) GetTimeout() int32 {
	timeout := int32(60)
	if s.Timeout != nil {
		timeout = *s.Timeout
	}
	return timeout
}

type ToAddSpec struct {
	Count *int32 `json:"count,omitempty"`
}

// ScaleOutStatus defines the observed state of ScaleOut
type ScaleOutStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	common.JobStatus `json:",inline"`

	AddPods []string `json:"addPods,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.conditions[-1:].type`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:object:root=true

// ScaleOut is the Schema for the scaleouts API
type ScaleOut struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ScaleOutSpec `json:"spec,omitempty"`
	// Most recently observed status of the PyTorchJob.
	// Read-only (modified by the system).
	Status ScaleOutStatus `json:"status,omitempty"`
}

type Selector struct {
	Name string `json:"name,omitempty"`
}

// +kubebuilder:object:root=true

// ScaleOutList contains a list of ScaleOut
type ScaleOutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScaleOut `json:"items"`
}

func (s *ScaleOut) GetJobStatus() *common.JobStatus {
	return &s.Status.JobStatus
}

func (s *ScaleOut) GetStatus() interface{} {
	return &s.Status
}

func (s *ScaleOut) GetFullName() string {
	return fmt.Sprintf("ScaleOut:%s/%s", s.Namespace, s.Name)
}

func (s *ScaleOut) GetScaleType() string {
	return SCALER_TYPE_SCALEOUT
}

func (s *ScaleOut) GetPodNames() []string {
	return s.Status.AddPods
}

func (s *ScaleOut) GetSelector() Selector {
	return s.Spec.Selector
}

func (s *ScaleOut) GetScriptSpec() ScaleScriptSpec {
	return s.Spec.ScaleScriptSpec
}

func init() {
	SchemeBuilder.Register(&ScaleOut{}, &ScaleOutList{})
}

package controllers

import (
	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	common "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ControllerInterface defines the Interface to be implemented by custom operators. e.g. tf-operator needs to implement this interface
type ControllerInterface interface {
	// Returns the Controller name
	ControllerName() string

	GetOrCreateSecret(job interface{}) (*corev1.Secret, error)

	GetOrCreateConfigMap(job interface{}, workerReplicas int32) (*corev1.ConfigMap, error)

	GetOrCreateLauncherServiceAccount(job interface{}) (*corev1.ServiceAccount, error)

	GetOrCreateLauncherRole(job interface{}, workerReplicas int32) (*rbacv1.Role, error)

	GetLauncherRoleBinding(job interface{}) (*rbacv1.RoleBinding, error)

	GetOrCreateWorker(job interface{}) ([]*corev1.Pod, error)

	CreateLauncher(job interface{}) (*corev1.Pod, error)

	GetLauncherJob(job interface{}) (*corev1.Pod, error)

	GetWorkerPods(job interface{}) ([]*corev1.Pod, error)

	DeleteWorkerPods(job interface{}) error
}

type Scaler interface {
	RuntimeObject
	GetFullName() string
	GetSelector() kaiv1alpha1.Selector
	GetScriptSpec() kaiv1alpha1.ScaleScriptSpec
	GetPodNames() []string
	GetScaleType() string
}

type RuntimeObject interface {
	runtime.Object
	v1.Object
	GetJobStatus() *common.JobStatus
	GetStatus() interface{}
}

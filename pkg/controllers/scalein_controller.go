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

package controllers

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"time"

	"github.com/go-logr/logr"
	logger "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"fmt"
	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	"github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	scaleInControllerName = "scalein"
)

// ScaleInReconciler reconciles a ScaleIn object
type ScaleInReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
	// BackoffStatesQueue is a rate limited queue and record backoff counts for
	// those reconciling-failed job instances, and it does not play a role of
	// build-in work queue in controller-runtime.
	BackoffStatesQueue workqueue.RateLimitingInterface
	PollInterval       time.Duration
}

func NewScaleInReconciler(mgr ctrl.Manager, pollInterval time.Duration) *ScaleInReconciler {
	r := &ScaleInReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		PollInterval:       pollInterval,
		Log:                ctrl.Log.WithName("controllers").WithName("ScaleIn"),
		BackoffStatesQueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
	r.recorder = mgr.GetEventRecorderFor(scaleInControllerName)
	return r
}

// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kai.alibabacloud.com,resources=scaleins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kai.alibabacloud.com,resources=scaleins/status,verbs=get;update;patch

func (r *ScaleInReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	//silog := r.Log.WithValues("scalein", req.NamespacedName)
	scaleIn, err := getScaleIn(req.NamespacedName, r.Client)
	if err != nil {
		// Error reading the object - requeue the request.
		return RequeueImmediately()
	}

	if scaleIn == nil || scaleIn.DeletionTimestamp != nil {
		logger.Infof("reconcile cancelled, scaleIn %s does not need to do reconcile or has been deleted", req.NamespacedName.String())
		return NoRequeue()
	}

	if isScaleFinished(*scaleIn.GetJobStatus()) {
		return NoRequeue()
	}

	//jobStatus := scaleIn.GetJobStatus()
	//if isScaleFinished(*jobStatus) {
	//	fixScaleStatus(r.Client, scaleIn)
	//	return NoRequeue()
	//}

	return setScalingOwner(r, scaleIn, r.PollInterval)
}

func (r *ScaleInReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiv1alpha1.ScaleIn{}).
		Complete(r)
}

func setScalingOwner(r client.Client, scaler Scaler, pollInterval time.Duration) (ctrl.Result, error) {
	ownerRefs := scaler.GetOwnerReferences()
	if len(ownerRefs) == 0 {
		trainingJob := &kaiv1alpha1.TrainingJob{}
		nsn := types.NamespacedName{}
		nsn.Namespace = scaler.GetNamespace()
		nsn.Name = scaler.GetSelector().Name
		err := r.Get(context.Background(), nsn, trainingJob)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Warnf("%s not found trainingjob: %v, namespace: %v", scaler.GetFullName(), nsn.Name, nsn.Namespace)
				return NoRequeue()
			}
			// Error reading the scaler - requeue the request.
			return RequeueAfterInterval(pollInterval, nil)
		}
		gvk := kaiv1alpha1.SchemeGroupVersionKind
		ownerRefs = append(ownerRefs, *metav1.NewControllerRef(trainingJob, schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind}))
		scaler.SetOwnerReferences(ownerRefs)
		msg := fmt.Sprintf("Scaling job %s created, match the training job %s", scaler.GetFullName(), nsn.String())

		initializeJobStatus(scaler.GetJobStatus())
		updateJobConditions(scaler.GetJobStatus(), v1.JobCreated, "", msg)
		err = r.Status().Update(context.Background(), scaler)
		if err != nil {
			logger.Errorf("failed to update scaler status: %s, err: %++v", scaler.GetFullName(), err)
			// Error updating the scaler - requeue the request.
			return RequeueAfterInterval(pollInterval, nil)
		}

		err = r.Update(context.Background(), scaler)
		if err != nil {
			logger.Errorf("failed to update scaler: %s, err: %++v", scaler.GetFullName(), err)
			// Error updating the scaler - requeue the request.
			return RequeueAfterInterval(pollInterval, nil)
		}
	}
	return NoRequeue()
}

func fixScaleStatus(r client.Client, scaler Scaler) {
	status := scaler.GetJobStatus()
	if status.Phase == v1.Scaling {
		if isScaleSucceeded(*status) {
			updateStatusPhase(status, v1.ScaleSucceeded)
		} else if isScaleFailed(*status) {
			updateStatusPhase(status, v1.ScaleFailed)
		}
	}
	updateObjectStatus(r, scaler, nil)

}

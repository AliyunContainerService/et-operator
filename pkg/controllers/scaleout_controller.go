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
	"fmt"
	"github.com/AliyunContainerService/et-operator/pkg/util"
	"github.com/go-logr/logr"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
)

const (
	scaleOutControllerName = "scaleout"
	scaleOutName           = "scaleout-name"
)

// ScaleOutReconciler reconciles a ScaleOut object
type ScaleOutReconciler struct {
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

func NewScaleOutReconciler(mgr ctrl.Manager, pollInterval time.Duration) *ScaleOutReconciler {
	r := &ScaleOutReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		PollInterval:       pollInterval,
		Log:                ctrl.Log.WithName("controllers").WithName("ScaleOut"),
		BackoffStatesQueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
	r.recorder = mgr.GetEventRecorderFor(scaleOutControllerName)
	return r
}

// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kai.alibabacloud.com,resources=scaleouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kai.alibabacloud.com,resources=scaleouts/status,verbs=get;update;patch

func (r *ScaleOutReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	scaleOut, err := getScaleOut(req.NamespacedName, r.Client)
	if err != nil {
		// Error reading the object - requeue the request.
		return RequeueImmediately()
	}
	if scaleOut == nil || scaleOut.DeletionTimestamp != nil {
		logger.Infof("reconcile cancelled, scaleOut %s does not need to do reconcile or has been deleted", req.NamespacedName.String())
		return NoRequeue()
	}

	if isScaleFinished(*scaleOut.GetJobStatus()) {
		return NoRequeue()
	}

	//jobStatus := scaleOut.GetJobStatus()
	//if isScaleFinished(*jobStatus) {
	//	fixScaleStatus(r.Client, scaleOut)
	//	return NoRequeue()
	//}

	return setScalingOwner(r, scaleOut, r.PollInterval)
}
func (r *ScaleOutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaiv1alpha1.ScaleOut{}).
		Complete(r)
}

func parseEnvs(envs []kaiv1alpha1.EnvSpec) (result string) {
	result = ""
	for i := 0; i < len(envs); i++ {
		result += fmt.Sprintf("export %s=%s ", envs[i].Name, envs[i].Value)
	}
	logger.Infof("parse envs: %v", result)
	return result
}

func scalerScript(timeout int32, envs []kaiv1alpha1.EnvSpec, scriptFile string, workers []string, slots int) string {
	var cmdStr string = ""
	if envs != nil && len(envs) != 0 {
		cmdStr = fmt.Sprintf("%s %s", parseEnvs(envs), "&&")
	}
	workerList := ""
	for i := 0; i < len(workers); i++ {
		// format workerList: pod1:slots,pod2:slots,...
		if i != len(workers)-1 {
			workerList += fmt.Sprintf("%s:%d,", workers[i], slots)
		} else {
			workerList += fmt.Sprintf("%s:%d", workers[i], slots)
		}
	}
	return fmt.Sprintf("%s timeout %d sh %s %s", cmdStr, timeout, scriptFile, workerList)
}

func hostfileUpdateScript(hostfile string, workers []string, slot int) string {
	return fmt.Sprintf(
		`echo '%s' > %s`, getHostfileContent(workers, slot), hostfile)
}

func kubectlOnPod(pod *corev1.Pod, cmd string) (string, string, error) {
	cmds := []string{
		"/bin/sh",
		"-c",
		cmd,
	}
	stdout, stderr, err := util.ExecCommandInContainerWithFullOutput(pod.Name, pod.Spec.Containers[0].Name, pod.Namespace, cmds)
	if err != nil {
		logger.Errorf("launcher execute command has error: %v, stdout:%s, stderr: %s", err, stdout, stderr)
		return stdout, stderr, err
	}
	return stdout, stderr, nil
}

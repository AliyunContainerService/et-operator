package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilpointer "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	common "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	"github.com/AliyunContainerService/et-operator/pkg/util"
)

func (r *TrainingJobReconciler) createTrainingJobWorkers(job *kaiv1alpha1.TrainingJob) error {

	if r.EnableCreateSecret && job.GetAttachMode() == kaiv1alpha1.AttachModeSSH {
		if cm, err := r.GetOrCreateSecret(job); cm == nil || err != nil {
			msg := fmt.Sprintf("job(%s/%s) create secret failed, error: %v", job.Namespace, job.Name, err)
			logger.Warn(msg)
			updateStatus(job.GetJobStatus(), common.JobFailed, trainingJobFailedReason, msg)
			return nil
		}
	}

	workers := getJobReplicasWorkers(job)
	job.Status.TargetWorkers = workers
	if err := r.CreateWorkers(job, workers); err != nil {
		msg := fmt.Sprintf("job(%s/%s) create workerPods failed, error: %v", job.Namespace, job.Name, err)
		logger.Warn(msg)
		updateStatus(job.GetJobStatus(), common.JobFailed, trainingJobFailedReason, msg)
		return nil
	}
	msg := fmt.Sprintf("create trainingjob(%v/%v) workers", job.Namespace, job.Name)
	logger.Infof(msg)
	updateJobConditions(job.GetJobStatus(), common.WorkersCreated, "", msg)
	return nil
}

func (r *TrainingJobReconciler) syncWorkersState(job *kaiv1alpha1.TrainingJob) error {
	workers, err := r.GetWorkerPods(job)
	if err != nil {
		return err
	}

	initializeJobStatus(job.GetJobStatus())

	r.workerReplicasStatus(job.GetJobStatus(), workers)

	err = r.handleWorkersFailed(job, workers)
	if err != nil {
		return err
	}
	return nil

}

func (r *TrainingJobReconciler) waitWorkersRunning(job *kaiv1alpha1.TrainingJob) error {
	if err := r.syncWorkersState(job); err != nil {
		return err
	}

	running := len(job.Status.CurrentWorkers)
	if running >= int(*job.Spec.ETReplicaSpecs.Worker.Replicas) {
		msg := fmt.Sprintf("trainingjob(%s/%s) all workers (%d) are running", job.Namespace, job.Name, running)
		logger.Infof(msg)
		updateJobConditions(job.GetJobStatus(), common.WorkersReady, "", msg)
	}
	return nil
}

func (r *TrainingJobReconciler) handleWorkersFailed(job *kaiv1alpha1.TrainingJob, pods []corev1.Pod) error {
	lastRunningPods := map[string]bool{}
	for _, worker := range job.Status.CurrentWorkers {
		lastRunningPods[worker] = true
	}
	restartPods := []string{}
	currentWorkers := []string{}

	currentPods := map[string]corev1.Pod{}
	for i, _ := range pods {
		pod := pods[i]
		currentPods[pod.Name] = pods[i]
		phase := pod.Status.Phase
		switch phase {
		case corev1.PodRunning:
			currentWorkers = append(currentWorkers, pod.Name)
		case corev1.PodFailed, corev1.PodUnknown, corev1.PodSucceeded:
			// only send event once
			if _, ok := lastRunningPods[pod.Name]; ok {
				msg := fmt.Sprintf("trainingjob(%s/%s): worker %s phase is %s.", job.Namespace, job.Name, pod.Name, pod.Status.Phase)
				if pod.Status.Reason == "Evicted" {
					msg = fmt.Sprintf("trainingjob(%s/%s): worker %s are evicted.", job.Namespace, job.Name, pod.Name)
				}
				r.recorder.Event(job, corev1.EventTypeWarning, trainingJobWorkerException, msg)
				logger.Infof(msg)
			}
		case corev1.PodPending:
		}
	}

	currentWorkers = sortPodNames(job.Name, currentWorkers)
	currentWorkersChange := strings.Compare(strings.Join(currentWorkers, ","),
		strings.Join(job.Status.CurrentWorkers, ",")) != 0
	logger.Infof("trainingjob(%v/%v) old current workers: %s", job.Namespace, job.Name, strings.Join(job.Status.CurrentWorkers, ","))
	logger.Infof("trainingjob(%v/%v) new current workers: %s", job.Namespace, job.Name, strings.Join(currentWorkers, ","))

	// update hostfile when find workers state updated
	if currentWorkersChange && hasCondition(*job.GetJobStatus(), common.JobRunning) {
		script := hostfileUpdateScript(getHostfilePath(job), currentWorkers, getSlots(job))
		if _, _, err := r.executeOnLauncher(job, script); err != nil {
			return err
		}
	}

	if len(job.Status.TargetWorkers) == 0 {
		job.Status.TargetWorkers = currentWorkers
	}
	job.Status.CurrentWorkers = currentWorkers
	job.Status.Replicas = int32(len(currentWorkers))

	// pod not exist in apiServer but in job workers
	// for example: delete a worker
	restartPolicy := jobRestartPolicy(job)
	for _, name := range job.Status.TargetWorkers {
		if pod, ok := currentPods[name]; ok {
			// deal pod fail
			if podNeedRestart(restartPolicy, pod) {
				restartPods = append(restartPods, pod.Name)
			}
		} else { // deal worker in  TargetWorkers，not in apiserver
			if restartPolicy == common.RestartPolicyAlways {
				restartPods = append(restartPods, name)
			}
			msg := fmt.Sprintf("trainingjob(%s/%s): worker %s phase is %s.", job.Namespace, job.Name, name, "Deleted")
			r.recorder.Event(job, corev1.EventTypeWarning, trainingJobWorkerException, msg)
			logger.Infof(msg)
		}
	}

	// ReCreate Failed or Successed Pods
	if len(restartPods) != 0 {
		maxIndex := getWorkerPodsMaxIndex(job.Status.TargetWorkers)
		createPods := []string{}
		for i := 0; i < len(restartPods); i++ {
			index := maxIndex + 1 + i
			createPods = append(createPods, getWorkerName(job.Name, index))
		}
		msg := fmt.Sprintf("trainingjob(%s/%s): recreate workers %++v for failed workers", job.Namespace, job.Name, createPods)
		if err := r.CreateWorkers(job, createPods); err != nil {
			msg := fmt.Sprintf("trainingjob(%s/%s): failed to create workers %++v.", job.Namespace, job.Name, createPods)
			r.recorder.Event(job, corev1.EventTypeWarning, trainingJobWorkerException, msg)
			logger.Infof(msg)
			return err
		}
		r.recorder.Event(job, corev1.EventTypeNormal, trainingJobWorkerCreated, msg)
		// replace  failed pod with new pod in TargetWorkers
		job.Status.TargetWorkers = append(filterNames(job.Status.TargetWorkers, restartPods, true), createPods...)
	}

	return nil
}

func jobRestartPolicy(job *kaiv1alpha1.TrainingJob) common.RestartPolicy {
	if job.Spec.ETReplicaSpecs.Worker.RestartPolicy == nil {
		//return common.RestartPolicyAlways
		return common.RestartPolicyNever
	}
	return *job.Spec.ETReplicaSpecs.Worker.RestartPolicy
}

func podNeedRestart(restartPolicy common.RestartPolicy, pod corev1.Pod) bool {
	switch restartPolicy {
	case common.RestartPolicyNever:
		return false
	case common.RestartPolicyAlways:
		return pod.Status.Phase == corev1.PodFailed ||
			pod.Status.Phase == corev1.PodUnknown ||
			pod.Status.Phase == corev1.PodSucceeded
	case common.RestartPolicyOnFailure:
		return pod.Status.Phase == corev1.PodFailed
	case common.RestartPolicyExitCode:
		if pod.Status.Phase == corev1.PodFailed {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if terminated := containerStatus.State.Terminated; terminated != nil {
					// 128-255: retryable error, will restart the pod.
					return terminated.ExitCode > 128 && terminated.ExitCode < 255
				}
			}
		}
	}
	return false
}

func workerReplicaStatuses(replicaStatus *common.ReplicaStatus, pods []corev1.Pod) (running, evict int) {
	for i := 0; i < len(pods); i++ {
		//logger.Infof("worker %v: name: %v", i, workers[i].Name)
		switch pods[i].Status.Phase {
		case corev1.PodFailed:
			replicaStatus.Failed += 1
			if pods[i].Status.Reason == "Evicted" {
				evict += 1
			}
		case corev1.PodSucceeded:
			replicaStatus.Succeeded += 1

		case corev1.PodRunning:
			running += 1
			replicaStatus.Active += 1
		}
	}
	return
}

func (r *TrainingJobReconciler) ScaleOutWorkers(job *kaiv1alpha1.TrainingJob, scaleout *kaiv1alpha1.ScaleOut) error {
	return r.createWorkers(job, scaleout.Status.AddPods, func(name string, index string) *corev1.Pod {
		worker := newWorker(job, name, index)
		worker.Labels[scaleOutName] = scaleout.Name
		return worker
	})
}

func (r *TrainingJobReconciler) CreateWorkers(job *kaiv1alpha1.TrainingJob, workers []string) error {
	return r.createWorkers(job, workers, func(name string, index string) *corev1.Pod {
		worker := newWorker(job, name, index)
		return worker
	})
}

func (r *TrainingJobReconciler) createWorkers(job *kaiv1alpha1.TrainingJob, workers []string, newPod PodTplGenerator) error {
	for _, podName := range workers {
		index, err := getWorkerIndex(job.Name, podName)
		if err != nil {
			logger.Infof("trainingjob(%v/%v) fail to getWorkerIndex: %++v ", job.Namespace, job.Name, err)
			return err
		}
		_, err = r.createWorker(job, int32(index), newPod)
		if err != nil {
			logger.Infof("trainingjob(%v/%v) fail to createWorker: %++v ", job.Namespace, job.Name, err)
			return err
		}
	}
	logger.Infof("Success trainingjob(%v/%v) createWorkers", job.Namespace, job.Name)
	return nil
}

type PodTplGenerator func(name string, index string) *corev1.Pod

func getWorkerName(jobName string, index int) string {
	workerPrefix := jobName + workerSuffix
	return fmt.Sprintf("%s-%s", workerPrefix, strconv.Itoa(int(index)))
}
func getWorkerIndex(jobName string, workerName string) (int64, error) {
	workerPrefix := jobName + workerSuffix
	indexStr := strings.TrimPrefix(workerName, fmt.Sprintf("%s-", workerPrefix))
	return strconv.ParseInt(indexStr, 10, 64)
}

func (r *TrainingJobReconciler) createWorker(job *kaiv1alpha1.TrainingJob, index int32, workerPodTempl PodTplGenerator) (*corev1.Pod, error) {
	name := getWorkerName(job.Name, int(index))
	indexStr := strconv.Itoa(int(index))
	pod := &corev1.Pod{}
	nsn := types.NamespacedName{
		Name:      name,
		Namespace: job.Namespace,
	}
	err := r.Get(context.Background(), nsn, pod)

	if err != nil {
		// If the worker Pod doesn't exist, we'll create it.
		if errors.IsNotFound(err) {
			worker := workerPodTempl(name, indexStr)
			if job.GetAttachMode() == kaiv1alpha1.AttachModeSSH {
				secretName := job.Name
				if name, ok := job.Annotations[common.SSHSecretName]; ok && name != "" {
					secretName = name
				}
				util.MountRsaKey(worker, secretName)
			}
			if err = r.Create(context.Background(), worker); err != nil {
				r.recorder.Eventf(job, corev1.EventTypeWarning, trainingJobFailedReason, "worker pod created failed: %v", err)
				return nil, err
			}
			logger.Infof("success to create pod: %s/%s", name, job.Namespace)
		} else {
			return nil, err
		}
	}

	service := &corev1.Service{}
	err = r.Get(context.Background(), nsn, service)
	if errors.IsNotFound(err) {
		err = r.Create(context.Background(), newService(job, name, indexStr))
	}
	if err != nil {
		r.recorder.Eventf(job, corev1.EventTypeWarning, trainingJobFailedReason, "worker service created failed: %v", err)
		return nil, err
	}
	return nil, nil
}

func (r *TrainingJobReconciler) GetWorkerPods(job *kaiv1alpha1.TrainingJob) ([]corev1.Pod, error) {
	return r.getWorkerPods(job, nil)
}

func (r *TrainingJobReconciler) getScalerOutExcludeWorkers(job *kaiv1alpha1.TrainingJob, scaleOut *kaiv1alpha1.ScaleOut) ([]corev1.Pod, error) {
	labelSelectorRequirement := metav1.LabelSelectorRequirement{
		Key:      scaleOutName,
		Operator: metav1.LabelSelectorOpNotIn,
		Values:   []string{scaleOut.GetName()},
	}
	fn := func(labelSelector *metav1.LabelSelector) *metav1.LabelSelector {
		labelSelector.MatchExpressions = []metav1.LabelSelectorRequirement{labelSelectorRequirement}
		return labelSelector
	}
	return r.getWorkerPods(job, fn)
}

func (r *TrainingJobReconciler) getScalerOutWorkers(job *kaiv1alpha1.TrainingJob, scaleOut *kaiv1alpha1.ScaleOut) ([]corev1.Pod, error) {
	fn := func(labelSelector *metav1.LabelSelector) *metav1.LabelSelector {
		labelSelector.MatchLabels[scaleOutName] = scaleOut.GetName()
		return labelSelector
	}
	return r.getWorkerPods(job, fn)
}

func (r *TrainingJobReconciler) getWorkerPods(job *kaiv1alpha1.TrainingJob, labelSelectorFunc func(labelSelector *metav1.LabelSelector) *metav1.LabelSelector) ([]corev1.Pod, error) {
	workerLabels := GenLabels(job.GetName())
	workerLabels[labelTrainingRoleType] = worker
	labelSelector := &metav1.LabelSelector{
		MatchLabels: workerLabels,
	}
	if labelSelectorFunc != nil {
		labelSelector = labelSelectorFunc(labelSelector)
	}

	selector, err := metav1.LabelSelectorAsSelector(labelSelector)

	podlist := &corev1.PodList{}
	err = r.List(context.Background(), podlist, client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}
	return podlist.Items, nil
}

func (r *TrainingJobReconciler) GetWorkerServices(job *kaiv1alpha1.TrainingJob) ([]corev1.Service, error) {
	workerLabels := GenLabels(job.Name)
	workerLabels[labelTrainingRoleType] = worker
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: workerLabels,
	})
	// List all pods to include those that don't match the selector anymore
	// but have a ControllerRef pointing to this controller.
	servicelist := &corev1.ServiceList{}
	if err = r.List(context.Background(), servicelist, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		logger.Warnf("get workerServices failed, error: %v", err)
		return nil, err
	}
	return servicelist.Items, nil
}

func (r *TrainingJobReconciler) DeleteAllWorkerServices(job *kaiv1alpha1.TrainingJob) error {
	return r.DeleteWorkerServices(job, nil)
}

func (r *TrainingJobReconciler) DeleteWorkerServices(job *kaiv1alpha1.TrainingJob, workers []string) error {
	workerServices, err := r.GetWorkerServices(job)
	if err != nil {
		logger.Errorf("Get current workerPods failed, error: %v", err)
		return err
	}
	if len(workerServices) == 0 {
		return nil
	}
	if workers != nil {
		workerServices = filterServiceNames(workerServices, workers)
	}

	for _, svc := range workerServices {
		deleteOptions := &client.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(0)}
		if err := r.Delete(context.Background(), &svc, deleteOptions); err != nil && !errors.IsNotFound(err) {
			r.recorder.Eventf(job, corev1.EventTypeWarning, trainingJobFailedReason, "Error deleting: %v", err)
		}
		r.recorder.Eventf(job, corev1.EventTypeNormal, trainingJobSucceededReason, "Deleted service: %v", svc.Name)
	}
	logger.Infof("Success trainingjob(%v/%v) DeleteWorkerServices", job.Namespace, job.Name)

	return nil
}

func (r *TrainingJobReconciler) DeleteAllWorkerPods(job *kaiv1alpha1.TrainingJob) error {
	return r.DeleteWorkerPods(job, nil)
}

func (r *TrainingJobReconciler) DeleteWorkerPods(job *kaiv1alpha1.TrainingJob, pods []string) error {
	workerPods, err := r.GetWorkerPods(job)
	if err != nil {
		logger.Errorf("Get current workerPods failed, error: %v", err)
		return err
	}
	if len(workerPods) == 0 {
		return nil
	}
	if pods != nil {
		workerPods = filterPodNames(workerPods, pods, false)
	}
	for _, pod := range workerPods {
		// deleteOptions := &client.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(0)}
		if err := r.Delete(context.Background(), &pod); err != nil && !errors.IsNotFound(err) {
			r.recorder.Eventf(job, corev1.EventTypeWarning, trainingJobFailedReason, "Error deleting worker %s: %v", pod.Name, err)
			//return err
		}
		r.recorder.Eventf(job, corev1.EventTypeNormal, trainingJobSucceededReason, "Deleted pod %s", pod.Name)
	}
	logger.Infof("Success trainingjob(%v/%v) DeleteWorkerPods", job.Namespace, job.Name)
	return nil
}

// @Deprecated
func (r *TrainingJobReconciler) GetOrCreateSecret(job *kaiv1alpha1.TrainingJob) (*corev1.Secret, error) {
	s := &corev1.Secret{}
	req := ctrl.Request{}
	req.NamespacedName.Namespace = job.GetNamespace()
	req.NamespacedName.Name = job.GetName()
	err := r.Get(context.Background(), req.NamespacedName, s)
	if errors.IsNotFound(err) {
		s = newSecret(job)
		logger.Infof("%v will create secret", req.NamespacedName)
		r.Create(context.Background(), s)
	}
	return s, nil
}

func (r *TrainingJobReconciler) CreateHostConfigMap(job *kaiv1alpha1.TrainingJob) (*corev1.ConfigMap, error) {
	return r.createConfigMap(job, newHostfileConfigMap)
}

func (r *TrainingJobReconciler) createConfigMap(job *kaiv1alpha1.TrainingJob, newCm func(job *kaiv1alpha1.TrainingJob) *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	name := ctrl.Request{}
	name.NamespacedName.Namespace = job.GetNamespace()
	name.NamespacedName.Name = job.GetName() + configSuffix
	err := r.Get(context.Background(), name.NamespacedName, cm)
	if errors.IsNotFound(err) {
		if err = r.Create(context.Background(), newCm(job)); err != nil {
			return cm, err
		}
	}
	return cm, nil
}

func getJobReplicasWorkers(job *kaiv1alpha1.TrainingJob) []string {
	if len(job.Status.TargetWorkers) != 0 {
		return job.Status.TargetWorkers
	}
	var i int32 = 0
	workers := []string{}
	workerReplicas := *job.Spec.ETReplicaSpecs.Worker.Replicas

	for ; i < workerReplicas; i++ {
		workers = append(workers, getWorkerName(job.Name, int(i)))
	}
	return workers
}

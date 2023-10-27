package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *TrainingJobReconciler) executeScaleIn(job *kaiv1alpha1.TrainingJob, scaleIn *kaiv1alpha1.ScaleIn) error {
	if scaleIn.DeletionTimestamp != nil || isScaleFinished(*scaleIn.GetJobStatus()) {
		logger.Info("reconcile cancelled, scalein does not need to do reconcile or has been deleted")
		return nil
	}

	initializeJobStatus(scaleIn.GetJobStatus())

	//TODO: Validate the scalein count for minSize
	err := r.setsSaleInToDelete(job, scaleIn)
	if err != nil {
		msg := fmt.Sprintf("%s get to delete workers name failed, error: %v", scaleIn.GetFullName(), err)
		r.updateScalerFailed(scaleIn, job, msg)
		return nil
	}

	currentWorkers := r.workersAfterScaler(job.Status.CurrentWorkers, scaleIn)

	// execute scalein script
	if err := r.executeScaleScript(job, scaleIn, currentWorkers); err != nil {
		msg := fmt.Sprintf("%s execute script failed, error: %v", scaleIn.GetFullName(), err)
		r.updateScalerFailed(scaleIn, job, msg)
		return nil
	}

	toDeleteWorkers := scaleIn.GetPodNames()
	remainWorkers := false
	if scaleIn.Spec.Script == "" {
		if shutdownWorkers, err := r.checkWorkerShutdown(job, toDeleteWorkers); err != nil {
			return err
		} else {
			if len(toDeleteWorkers) != len(shutdownWorkers) {
				remainWorkers = true
				toDeleteWorkers = shutdownWorkers
			}
		}
	}
	if err := r.DeleteWorkers(job, toDeleteWorkers); err != nil {
		msg := fmt.Sprintf("%s delete resource failed, error: %v", scaleIn.GetFullName(), err)
		r.updateScalerFailed(scaleIn, job, msg)
		return nil
	}

	// wait pods deleted
	deleted, _ := r.isWorkersDeleted(job.Namespace, scaleIn.GetPodNames())
	if deleted {
		job.Status.TargetWorkers = r.workersAfterScaler(job.Status.TargetWorkers, scaleIn)
		job.Status.CurrentWorkers = currentWorkers
		job.Status.Replicas = int32(len(currentWorkers))
		r.updateScalerSuccessd(scaleIn, job)
		return nil
	}

	if remainWorkers {
		msg := "wait for workers process shutdown"
		logger.Info(msg)
		return NewRequeueError(fmt.Errorf(msg))
	}

	logger.Infof("wait for pods deleted")
	return nil
}

func (r *TrainingJobReconciler) DeleteWorkers(trainingJob *kaiv1alpha1.TrainingJob, workers []string) error {
	if err := r.DeleteWorkerServices(trainingJob, workers); err != nil {
		return fmt.Errorf("delete services failed: %++v", err)
	}

	if err := r.DeleteWorkerPods(trainingJob, workers); err != nil {
		return fmt.Errorf("delete pods failed: %++v", err)
	}
	return nil
}

func (r *TrainingJobReconciler) checkWorkerShutdown(job *kaiv1alpha1.TrainingJob, workers []string) ([]string, error) {
	result := []string{}
	attachCmd := "ssh -o PasswordAuthentication=no -o StrictHostKeyChecking=no"
	if job.GetAttachMode() == kaiv1alpha1.AttachModeKubexec {
		attachCmd = getKubexecPath()
	}
	out, _, err := r.executeOnLauncher(job, fmt.Sprintf("ps -ef | grep '%s'", attachCmd))
	if err != nil {
		logger.Errorf("failed to exec on %s, err: %++v", job.Name, err)
		return result, err
	}
	logger.Infof("launcher execute status is : %s", out)
	for _, pod := range workers {
		if strings.Contains(out, fmt.Sprintf("%s %s", attachCmd, pod)) {
			continue
		} else {
			result = append(result, pod)
		}
	}
	return result, nil
}

func (r *TrainingJobReconciler) isWorkersDeleted(namespace string, workersName []string) (bool, error) {
	if len(workersName) == 0 {
		return true, nil
	}
	for i := 0; i < len(workersName); i++ {
		nsn := types.NamespacedName{}
		nsn.Namespace = namespace
		nsn.Name = workersName[i]
		pod := &corev1.Pod{}
		if err := r.Get(context.Background(), nsn, pod); err != nil {
			if errors.IsNotFound(err) {
				logger.Infof("worker(%v) has been deleted.", nsn)
				continue
			} else {
				logger.Warnf("get pod: %v failed, error: %v", nsn, err)
				return false, err
			}
		}
		if pod != nil {
			return false, nil
		}
	}
	return true, nil
}

func (r *TrainingJobReconciler) setsSaleInToDelete(job *kaiv1alpha1.TrainingJob, scaleIn *kaiv1alpha1.ScaleIn) error {
	podNames := scaleIn.Status.ToDeletePods
	if len(podNames) != 0 {
		return /*filterPodNames(workers, podNames, false), */ nil
	}
	workers, err := r.GetWorkerPods(job)
	if err != nil {
		err = fmt.Errorf("get current order workers name failed: %v", err)
		return err
	}

	toDelete := scaleIn.Spec.ToDelete

	if toDelete.PodNames != nil {
		workers = filterPodNames(workers, toDelete.PodNames, false)
	} else if toDelete.Count > 0 {
		if toDelete.Count < len(workers) {
			allPodNames := getSortPodNames(job.Name, workers)
			deletePodNames := allPodNames[len(workers)-toDelete.Count:]
			workers = filterPodNames(workers, deletePodNames, false)
		} else {
			return fmt.Errorf(".spec.toDelete.count should be less than current replicas %d", len(workers))
		}
	} else {
		return fmt.Errorf(".spec.toDelete.count or .spec.toDelete.podNames, only one of them should be set")
	}
	for _, worker := range workers {
		scaleIn.Status.ToDeletePods = append(scaleIn.Status.ToDeletePods, worker.Name)
	}

	return nil
}

func filterNames(workers []string, names []string, invert bool) []string {
	result := []string{}
	for _, worker := range workers {
		if invert {
			if !containsString(names, worker) {
				result = append(result, worker)
			}
		} else {
			if containsString(names, worker) {
				result = append(result, worker)
			}
		}
	}
	return result
}

func filterPodNames(pods []corev1.Pod, names []string, invert bool) []corev1.Pod {
	result := []corev1.Pod{}
	for _, pod := range pods {
		if invert {
			if !containsString(names, pod.Name) {
				result = append(result, pod)
			}
		} else {
			if containsString(names, pod.Name) {
				result = append(result, pod)
			}
		}
	}
	return result
}

func filterServiceNames(services []corev1.Service, names []string) []corev1.Service {
	result := []corev1.Service{}
	for _, service := range services {
		if containsString(names, service.GetName()) {
			result = append(result, service)
		}
	}
	return result
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func getSortPodNames(jobName string, pods []corev1.Pod) []string {
	workersNames := []string{}
	for _, pod := range pods {
		workersNames = append(workersNames, pod.Name)
	}
	return sortPodNames(jobName, workersNames)
}

func sortPodNames(jobName string, workersNames []string) []string {
	workersIndexs := []int{}
	workersName := []string{}
	for _, name := range workersNames {
		workerIndex, err := getWorkerIndex(jobName, name)
		if err != nil {
			logger.Errorf("job %s worker %s name invalid: %++v", jobName, name, err)
			continue
		}
		workersIndexs = append(workersIndexs, int(workerIndex))
	}
	sort.Sort(sort.IntSlice(workersIndexs))
	for _, index := range workersIndexs {
		workersName = append(workersName, getWorkerName(jobName, index))
	}
	return workersName
}

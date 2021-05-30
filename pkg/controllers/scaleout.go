package controllers

import (
	"fmt"
	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	commonv1 "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	//"k8s.io/apimachinery/pkg/api/errors"
	"strconv"
	"strings"
)

func (r *TrainingJobReconciler) executeScaleOut(job *kaiv1alpha1.TrainingJob, scaleOut *kaiv1alpha1.ScaleOut) error {
	if scaleOut.DeletionTimestamp != nil {
		logger.Infof("reconcile cancelled, scaleOut %s does not need to do reconcile or has been deleted", scaleOut.GetFullName())
		return nil
	}

	initializeJobStatus(scaleOut.GetJobStatus())

	if err := r.validateScaleOut(job, scaleOut); err != nil {
		r.updateScalerAbort(scaleOut, job, err.Error())
		return nil
	}

	if err := r.setScaleOutWorkers(job, scaleOut); err != nil {
		return err
	}

	err := r.ScaleOutWorkers(job, scaleOut)
	if err != nil {
		msg := fmt.Sprintf("%s create scaleout workers failed, error: %v", scaleOut.GetFullName(), err)
		r.ScaleOutFailed(job, scaleOut, msg)
		return err
	}

	scaleOutWorkers, err := r.getScalerOutWorkers(job, scaleOut)
	if err != nil {
		return err
	}

	workerStatuses, _ := r.workerReplicasStatus(scaleOut.GetJobStatus(), scaleOutWorkers)

	if workerStatuses.Active < *scaleOut.Spec.ToAdd.Count {
		if IsScaleOutTimeout(scaleOut) {
			msg := fmt.Sprintf("scaleout job %s execution timeout", scaleOut.GetFullName())
			r.ScaleOutFailed(job, scaleOut, msg)
		}
		return NewRequeueError(fmt.Errorf("wait for workers running"))
	}

	hostWorkers := r.workersAfterScaler(job.Status.CurrentWorkers, scaleOut)

	// execute scalein script
	if err := r.executeScaleScript(job, scaleOut, hostWorkers); err != nil {
		msg := fmt.Sprintf("%s execute script failed, error: %v", scaleOut.GetFullName(), err)
		r.ScaleOutFailed(job, scaleOut, msg)
		return err
	} else {
		job.Status.TargetWorkers = r.workersAfterScaler(job.Status.TargetWorkers, scaleOut)
		r.updateScalerSuccessd(scaleOut, job)
	}

	return nil
}

func (r *TrainingJobReconciler) validateScaleOut(job *kaiv1alpha1.TrainingJob, scaleOut *kaiv1alpha1.ScaleOut) error {
	if scaleOut.Spec.ToAdd == nil || scaleOut.Spec.ToAdd.Count == nil {
		return fmt.Errorf(".spec.toAdd.count shouldn't be empty")
	}
	cnt := *scaleOut.Spec.ToAdd.Count
	if cnt <= 0 {
		return fmt.Errorf(".spec.toAdd.count shouldn be greater than 0")
	}
	return r.validateReplica(job, &cnt)
}

func (r *TrainingJobReconciler) ScaleOutFailed(job *kaiv1alpha1.TrainingJob, scaleOut *kaiv1alpha1.ScaleOut, msg string) error {
	r.updateScalerFailed(scaleOut, job, msg)
	return r.DeleteWorkers(job, scaleOut.GetPodNames())
}

func (r *TrainingJobReconciler) workerReplicasStatus(jobStatus *commonv1.JobStatus, workers []corev1.Pod) (*commonv1.ReplicaStatus, int) {
	workerStatuses := &commonv1.ReplicaStatus{}
	jobStatus.ReplicaStatuses[commonv1.ReplicaType(kaiv1alpha1.ETReplicaTypeWorker)] = workerStatuses

	_, evict := workerReplicaStatuses(workerStatuses, workers)
	return workerStatuses, evict
}

func (r *TrainingJobReconciler) setScaleOutWorkers(job *kaiv1alpha1.TrainingJob, scaleOut *kaiv1alpha1.ScaleOut) error {
	if len(scaleOut.Status.AddPods) != 0 {
		return nil
	}
	addCount := *scaleOut.Spec.ToAdd.Count

	//pods, err := r.getScalerOutExcludeWorkers(job, scaleOut)
	//if err != nil {
	//	return err
	//}
	maxIndex := getWorkerPodsMaxIndex(job.Status.TargetWorkers)

	for i := 0; i < int(addCount); i++ {
		index := i + maxIndex + 1
		scaleOut.Status.AddPods = append(scaleOut.Status.AddPods, getWorkerName(job.Name, index))
	}
	return nil
}

func getWorkerPodsMaxIndex(pods []string) int {
	if pods == nil || len(pods) == 0 {
		return -1
	}
	maxIndex := 0
	for _, pod := range pods {
		temp := strings.Split(pod, "-")
		indexStr := temp[len(temp)-1]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			logger.Errorf("maxIndex: %v converted to type int failed, err: %v", temp, err)
			//return 0, err
		}
		if index > maxIndex {
			maxIndex = index
		}
	}

	return maxIndex
}

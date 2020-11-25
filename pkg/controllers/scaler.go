package controllers

import (
	"context"
	"fmt"
	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	common "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	commonv1 "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func (r *TrainingJobReconciler) executeScaling(job *kaiv1alpha1.TrainingJob) error {
	if err := r.syncLauncherState(job); err != nil {
		return err
	}

	if job.Status.CurrentScaler == "" {
		updateStatusPhase(job.GetJobStatus(), common.JobRunning)
		return nil
	}

	if isFinished(*job.GetJobStatus()) {
		return nil
	}

	scalerType, scalerName := getScalerName(job.Status.CurrentScaler)
	if scalerType == "ScaleIn" {
		scaleIn, err := getScaleIn(scalerName, r)
		if err != nil {
			return err
		}
		if scaleIn == nil || isScaleFinished(*scaleIn.GetJobStatus()) {
			finishTrainingScaler(job.GetJobStatus())
			return nil
		}

		oldStatus := scaleIn.Status.DeepCopy()
		defer r.updateObjectStatus(scaleIn, oldStatus)

		if err = r.executeScaleIn(job, scaleIn); err != nil {
			if !IsRequeueError(err) {
				logger.Errorf("job %s execute scaleIn %s failed, err: %++v", job.Name, scaleIn.GetFullName(), err)
			}
			return err
		}
	} else if scalerType == "ScaleOut" {
		scaleOut, err := getScaleOut(scalerName, r)
		if err != nil {
			return err
		}
		if scaleOut == nil || isScaleFinished(*scaleOut.GetJobStatus()) {
			finishTrainingScaler(job.GetJobStatus())
			return nil
		}

		oldStatus := scaleOut.Status.DeepCopy()
		defer r.updateObjectStatus(scaleOut, oldStatus)

		if err = r.executeScaleOut(job, scaleOut); err != nil {
			if !IsRequeueError(err) {
				logger.Errorf("job %s execute scaleOut %s failed, err: %++v", job.Name, scaleOut.GetFullName(), err)
			}
			return err
		}
	}
	return nil
}

func (r *TrainingJobReconciler) setTrainingJobScaler(job *kaiv1alpha1.TrainingJob) error {
	scaleOut, err := r.availableScaleOutList(job)
	if err != nil {
		logger.Warnf("list scaleOut failed, err: %v", err)
	}

	scaleIn, err := r.availableScaleInList(job)
	if err != nil {
		logger.Warnf("list scaleOut failed, err: %v", err)
	}

	scalerList := append(scaleOut, scaleIn...)

	// Select the latest scaling job
	r.updateLatestScaler(job, scalerList)
	return nil
}

func (r *TrainingJobReconciler) availableScaleOutList(job *kaiv1alpha1.TrainingJob) ([]Scaler, error) {
	scaleOutList := &kaiv1alpha1.ScaleOutList{}
	if err := r.Client.List(context.TODO(), scaleOutList); err != nil {
		logger.Warnf("list scaleOut failed, err: %v", err)
		return nil, err
	}
	scalerList := []Scaler{}
	for i, _ := range scaleOutList.Items {
		scaleOutItem := scaleOutList.Items[i]
		if filterAvailableScaler(&scaleOutItem, job) {
			scalerList = append(scalerList, &scaleOutItem)
		}
	}
	return scalerList, nil
}

func (r *TrainingJobReconciler) availableScaleInList(job *kaiv1alpha1.TrainingJob) ([]Scaler, error) {
	scaleInList := &kaiv1alpha1.ScaleInList{}
	if err := r.Client.List(context.TODO(), scaleInList); err != nil {
		logger.Warnf("list scaleIn failed, err: %v", err)
		return nil, err
	}

	scalerList := []Scaler{}
	for i, _ := range scaleInList.Items {
		scaleInItem := scaleInList.Items[i]
		if filterAvailableScaler(&scaleInItem, job) {
			scalerList = append(scalerList, &scaleInItem)
		}
	}
	return scalerList, nil
}

func (r *TrainingJobReconciler) executeScaleScript(trainingJob *kaiv1alpha1.TrainingJob, scaler Scaler, workers []string) error {
	if isScriptExecuted(*scaler.GetJobStatus()) {
		return nil
	}
	msg := fmt.Sprintf("trainingjob(%s/%s): execute script on launcher for %s", trainingJob.Namespace, trainingJob.Name, scaler.GetFullName())
	logger.Info(msg)

	slots := getSlots(trainingJob)
	scriptSpec := scaler.GetScriptSpec()

	var script string
	if scriptSpec.Script != "" {
		script = scalerScript(scriptSpec.GetTimeout(), scriptSpec.Env, scriptSpec.Script, scaler.GetPodNames(), slots)
	} else {
		hostfilePath := getHostfilePath(trainingJob)
		script = hostfileUpdateScript(hostfilePath, workers, slots)
	}

	_, _, err := r.executeOnLauncher(trainingJob, script)
	if err != nil {
		return err
	}

	updateJobConditions(scaler.GetJobStatus(), commonv1.ScriptExecuted, "", msg)
	return nil
}

func (r *TrainingJobReconciler) workersAfterScaler(workers []string, scaler Scaler) []string {
	result := []string{}
	pods := scaler.GetPodNames()
	if scaler.GetScaleType() == kaiv1alpha1.SCALER_TYPE_SCALEOUT {
		result = append(workers, pods...)
	} else {
		for _, worker := range workers {
			if containsString(pods, worker) {
				continue
			}
			result = append(result, worker)
		}
	}
	return result
}

func (r *TrainingJobReconciler) updateLatestScaler(job *kaiv1alpha1.TrainingJob, scalers []Scaler) error {
	var latestScaler Scaler
	if len(scalers) == 0 {
		return nil
	}
	for i, _ := range scalers {
		scalerItem := scalers[i]
		if latestScaler == nil || latestScaler.GetCreationTimestamp().Time.Before(scalerItem.GetCreationTimestamp().Time) {
			latestScaler = scalerItem
		}
	}
	return r.updateCurrentScaler(job, latestScaler)
}

func (r *TrainingJobReconciler) updateCurrentScaler(job *kaiv1alpha1.TrainingJob, scaleItem Scaler) error {
	job.Status.CurrentScaler = scaleItem.GetFullName()
	msg := fmt.Sprintf("trainingJobob(%s/%s) execute %s", job.Namespace, job.Name, scaleItem.GetFullName())
	r.updateScalerState(scaleItem, job, newCondition(commonv1.Scaling, scalingStartReason, msg))

	if err := r.updateObjectStatus(scaleItem, nil); err != nil {
		return err
	}
	return nil
}

func (r *TrainingJobReconciler) updateScalerFailed(scaleObj Scaler, trainingJob *kaiv1alpha1.TrainingJob, msg string) error {
	logger.Error(msg)
	r.recorder.Event(scaleObj, corev1.EventTypeWarning, "", msg)
	reason := fmt.Sprintf("%s%s", scaleObj.GetScaleType(), commonv1.JobFailed)
	return r.updateScalerState(scaleObj, trainingJob, newCondition(commonv1.ScaleFailed, reason, msg))
}

func (r *TrainingJobReconciler) updateScalerSuccessd(scaleObj Scaler, trainingJob *kaiv1alpha1.TrainingJob) error {
	msg := fmt.Sprintf("%s has Succeeded", scaleObj.GetFullName())
	r.recorder.Event(scaleObj, corev1.EventTypeNormal, "", msg)
	reason := fmt.Sprintf("%s%s", scaleObj.GetScaleType(), commonv1.JobSucceeded)
	return r.updateScalerState(scaleObj, trainingJob, newCondition(commonv1.ScaleSucceeded, reason, msg))
}

func (r *TrainingJobReconciler) updateScalerState(scaleObj Scaler, trainingJob *kaiv1alpha1.TrainingJob, condition common.JobCondition) error {
	jobPhase := commonv1.Scaling
	currentJob := scaleObj.GetFullName()
	if condition.Type == commonv1.ScaleSucceeded || condition.Type == commonv1.ScaleFailed {
		jobPhase = commonv1.JobRunning
		currentJob = ""
	}

	setCondition(trainingJob.GetJobStatus(), condition)
	updateStatusPhase(trainingJob.GetJobStatus(), jobPhase)
	updateTrainingJobCurrentScaler(trainingJob.GetJobStatus(), currentJob)

	setCondition(scaleObj.GetJobStatus(), condition)
	updateStatusPhase(scaleObj.GetJobStatus(), condition.Type)

	return nil
}

func filterAvailableScaler(scaleItem Scaler, job *kaiv1alpha1.TrainingJob) bool {
	if isScaleFinished(*scaleItem.GetJobStatus()) {
		return false
	}
	return v1.IsControlledBy(scaleItem, job)
}

func getScaleIn(name types.NamespacedName, client client.Client) (*kaiv1alpha1.ScaleIn, error) {
	scaleIn := &kaiv1alpha1.ScaleIn{}
	if err := client.Get(context.TODO(), name, scaleIn); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("scaler scaleIn %s not found", name.String())
			return nil, nil
		}
		return nil, err
	}
	return scaleIn, nil
}

func getScaleOut(name types.NamespacedName, client client.Client) (*kaiv1alpha1.ScaleOut, error) {
	scaleOut := &kaiv1alpha1.ScaleOut{}
	if err := client.Get(context.TODO(), name, scaleOut); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("scaler scaleIn %s not found", name.String())
			return nil, nil
		}
		return nil, err
	}
	return scaleOut, nil
}

func getScalerName(n string) (scalerType string, name types.NamespacedName) {
	v := strings.Split(n, ":")
	if len(v) == 2 {
		return v[0], getNamespaceName(v[1])
	}
	return "", getNamespaceName(n)
}

func getNamespaceName(n string) types.NamespacedName {
	v := strings.Split(n, "/")
	if len(v) < 2 {
		return types.NamespacedName{
			Name: n,
		}
	}
	return types.NamespacedName{
		Namespace: v[0],
		Name:      v[1],
	}
}

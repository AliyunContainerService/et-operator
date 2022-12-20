package controllers

import (
	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	common "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

const (
	// trainingJobCreatedReason is added in a trainingjob when it is created.
	trainingJobCreatedReason = "TrainingJobCreated"
	// trainingJobSucceededReason is added in a trainingjob when it is succeeded.
	trainingJobSucceededReason = "TrainingJobSucceeded"
	// trainingJobRunningReason is added in a trainingjob when it is running.
	trainingJobRunningReason = "TrainingJobRunning"
	// trainingJobFailedReason is added in a trainingjob when it is failed.
	trainingJobFailedReason = "TrainingJobFailed"

	WaitingResourceTimeoutReason = "WaitingResourceTimeout"

	// trainingJobEvict
	trainingJobEvict = "TrainingJobEvicted"
	// trainingJobWorkerException
	trainingJobWorkerException = "TrainingJobWorkerFailed"
	// trainingJobWorkerCreated
	trainingJobWorkerCreated = "TrainingJobWorkerCreated"

	// scalingInCreatedReason is added in a scalein when it is created.
	scalingStartReason = "ScalingStart"
)

// initializeTrainingJobStatuses initializes the ReplicaStatuses for TrainingJob.
func initializeJobStatuses(jobStatus *common.JobStatus, rtype kaiv1alpha1.ETReplicaType) {
	initializeJobStatus(jobStatus)

	replicaType := common.ReplicaType(rtype)
	jobStatus.ReplicaStatuses[replicaType] = &common.ReplicaStatus{}
}

// initializeTrainingJobStatuses initializes the ReplicaStatuses for TrainingJob.
func initializeJobStatus(jobStatus *common.JobStatus) {
	if jobStatus.ReplicaStatuses == nil {
		jobStatus.ReplicaStatuses = make(map[common.ReplicaType]*common.ReplicaStatus)
	}
	if len(jobStatus.Conditions) == 0 {
		jobStatus.Conditions = []common.JobCondition{}
	}
}

func isScaleFinished(status common.JobStatus) bool {
	return isScaleSucceeded(status) || isScaleFailed(status)
}

func isScaleFailed(status common.JobStatus) bool {
	return hasCondition(status, common.ScaleFailed)
}
func isScaleSucceeded(status common.JobStatus) bool {
	return hasCondition(status, common.ScaleSucceeded)
}

func isScriptExecuted(status common.JobStatus) bool {
	return hasCondition(status, common.ScriptExecuted)
}

func isFinished(status common.JobStatus) bool {
	return isSucceeded(status) || isFailed(status)
}

// isSucceeded checks if the job is succeeded.
func isSucceeded(status common.JobStatus) bool {
	return hasCondition(status, common.JobSucceeded)
}

// isFailed checks if the job is failed.
func isFailed(status common.JobStatus) bool {
	return hasCondition(status, common.JobFailed)
}

func isEvicted(status common.JobStatus) bool {
	for _, condition := range status.Conditions {
		if condition.Type == common.JobFailed &&
			condition.Status == v1.ConditionTrue &&
			condition.Reason == trainingJobEvict {
			return true
		}
	}
	return false
}

// isRunning checks if the job is running.
func isRunning(status common.JobStatus) bool {
	return hasCondition(status, common.JobRunning)
}

func isCleanUpPods(cleanPodPolicy *common.CleanPodPolicy) bool {
	if *cleanPodPolicy == common.CleanPodPolicyAll || *cleanPodPolicy == common.CleanPodPolicyRunning {
		return true
	}
	return false
}

// updateJobConditions adds to the jobStatus a new condition if needed, with the conditionType, reason, and message.
func updateStatus(jobStatus *common.JobStatus, conditionType common.JobConditionType, reason, message string) error {
	updateJobConditions(jobStatus, conditionType, reason, message)
	updatePhase(jobStatus, conditionType)
	return nil
}

// updateJobConditions adds to the jobStatus a new condition if needed, with the conditionType, reason, and message.
func updateJobConditions(jobStatus *common.JobStatus, conditionType common.JobConditionType, reason, message string) error {
	condition := newCondition(conditionType, reason, message)
	setCondition(jobStatus, condition)
	return nil
}

// GetCondition returns the condition with the provided type.
func getCondition(status common.JobStatus, condType common.JobConditionType) *common.JobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == condType {
			return &condition
		}
	}
	return nil
}

func hasCondition(status common.JobStatus, condType common.JobConditionType) bool {
	for _, condition := range status.Conditions {
		if condition.Type == condType && condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

// newCondition creates a new job condition.
func newCondition(conditionType common.JobConditionType, reason, message string) common.JobCondition {
	return common.JobCondition{
		Type:               conditionType,
		Status:             v1.ConditionTrue,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// setCondition updates the job to include the provided condition.
// If the condition that we are about to add already exists
// and has the same status and reason then we are not going to update.
func setCondition(status *common.JobStatus, condition common.JobCondition) {
	// Do nothing if JobStatus have failed condition
	if isFailed(*status) {
		return
	}

	currentCond := getCondition(*status, condition.Type)

	// Do nothing if condition doesn't change
	if currentCond != nil && currentCond.Status == condition.Status &&
		currentCond.Reason == condition.Reason &&
		currentCond.Message == condition.Message {
		return
	}

	// Do not update lastTransitionTime if the status of the condition doesn't change.
	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}

	// Append the updated condition to the conditions
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, condition)
}

// filterOutCondition returns a new slice of job conditions without conditions with the provided type.
func filterOutCondition(conditions []common.JobCondition, condType common.JobConditionType) []common.JobCondition {
	var newConditions []common.JobCondition
	for _, c := range conditions {
		if condType == common.JobRestarting && c.Type == common.JobRunning {
			continue
		}
		if condType == common.JobRunning && c.Type == common.JobRestarting {
			continue
		}

		if c.Type == condType {
			continue
		}

		// Set the running condition status to be false when current condition failed or succeeded
		if (condType == common.JobFailed || condType == common.JobSucceeded) && c.Type == common.JobRunning {
			c.Status = v1.ConditionFalse
		}

		newConditions = append(newConditions, c)
	}
	return newConditions
}

func isPodFinished(j *v1.Pod) bool {
	return isPodSucceeded(j) || isPodFailed(j)
}

func isPodFailed(p *v1.Pod) bool {
	return p.Status.Phase == v1.PodFailed
}

func isPodSucceeded(p *v1.Pod) bool {
	return p.Status.Phase == v1.PodSucceeded
}

func isPodRunning(p *v1.Pod) bool {
	return p.Status.Phase == v1.PodRunning
}

func updatePhase(jobStatus *common.JobStatus, status common.JobConditionType) {
	//if jobStatus.Phase == common.Scaling {
	//	return
	//}
	jobStatus.Phase = status
}

func updateStatusPhase(jobStatus *common.JobStatus, status common.JobConditionType) {
	jobStatus.Phase = status
}

func updateTrainingJobCurrentScaler(jobStatus *common.JobStatus, name string) {
	jobStatus.CurrentScaler = name
}

func finishTrainingScaler(jobStatus *common.JobStatus) {
	updateTrainingJobCurrentScaler(jobStatus, "")
	updateStatusPhase(jobStatus, common.JobRunning)
}

func IsScaleOutTimeout(scaleout *kaiv1alpha1.ScaleOut) bool {
	timeout := scaleout.Spec.GetTimeout()
	for _, condition := range scaleout.Status.Conditions {
		if condition.Type == common.Scaling && condition.Reason == scalingStartReason {
			return condition.LastTransitionTime.Add(time.Duration(timeout) * time.Second).Before(time.Now())
		}
	}
	return false
}

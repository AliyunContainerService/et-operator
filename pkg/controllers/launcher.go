package controllers

import (
	"context"
	"fmt"

	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	commonv1 "github.com/AliyunContainerService/et-operator/pkg/controllers/api/v1"
	"github.com/AliyunContainerService/et-operator/pkg/util"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilpointer "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/meta"
)

func (r *TrainingJobReconciler) createLauncher(job *kaiv1alpha1.TrainingJob) error {
	if _, err := r.GetOrCreateLauncherServiceAccount(job); err != nil {
		msg := fmt.Sprintf("job(%s/%s) create serviceAccount failed, error: %v", job.Namespace, job.Name, err)
		logger.Warn(msg)
		updateStatus(job.GetJobStatus(), commonv1.JobFailed, trainingJobFailedReason, msg)
		return nil
	}
	if _, err := r.GetOrCreateLauncherRole(job, 0); err != nil {
		msg := fmt.Sprintf("job(%s/%s) create role failed, error: %v", job.Namespace, job.Name, err)
		logger.Warn(msg)
		updateStatus(job.GetJobStatus(), commonv1.JobFailed, trainingJobFailedReason, msg)
		return nil
	}
	if _, err := r.GetLauncherRoleBinding(job); err != nil {
		msg := fmt.Sprintf("job(%s/%s) create roleBinding failed, error: %v", job.Namespace, job.Name, err)
		logger.Warn(msg)
		updateStatus(job.GetJobStatus(), commonv1.JobFailed, trainingJobFailedReason, msg)
		return nil
	}

	if cm, err := r.CreateHostConfigMap(job); cm == nil || err != nil {
		msg := fmt.Sprintf("job(%s/%s) create configmap failed, error: %v", job.Namespace, job.Name, err)
		logger.Warn(msg)
		updateStatus(job.GetJobStatus(), commonv1.JobFailed, trainingJobFailedReason, msg)
		return nil
	}

	launcher, err := r.GetLauncherJob(job)
	if err != nil {
		logger.Warnf("get launcher failed, error: %v", err)
		return err
	}
	if launcher == nil {
		if _, err := r.CreateLauncher(job); err != nil {
			msg := fmt.Sprintf("job(%s/%s) create launcher failed, error: %v", job.Namespace, job.Name, err)
			logger.Warn(msg)
			updateStatus(job.GetJobStatus(), commonv1.JobFailed, trainingJobFailedReason, msg)
			return nil
		}
	}
	msg := fmt.Sprintf("create trainingjob(%v/%v) launcher", job.Namespace, job.Name)
	logger.Info(msg)
	updateJobConditions(job.GetJobStatus(), commonv1.LauncherCreated, "", msg)
	return nil
}

func (r *TrainingJobReconciler) reconcileJobRunning(job *kaiv1alpha1.TrainingJob) error {
	if err := r.syncLauncherState(job); err != nil {
		return err
	}
	if err := r.syncWorkersState(job); err != nil {
		return err
	}

	if job.Status.Phase == commonv1.JobRunning {
		return r.setTrainingJobScaler(job)
	}

	return nil
}

func (r *TrainingJobReconciler) syncLauncherState(job *kaiv1alpha1.TrainingJob) error {
	// Get the launcher Job for this job.If not found, will return nil. Other error will try again.
	launcher, err := r.GetLauncherJob(job)
	if err != nil {
		logger.Warnf("get launcher failed, error: %v", err)
		return err
	}
	if launcher == nil {
		logger.Warn("launcher not found")
		switch job.Spec.ETReplicaSpecs.Launcher.RestartPolicy {
		case commonv1.RestartPolicyAlways:
			if _, err := r.CreateLauncher(job); err != nil {
				msg := fmt.Sprintf("job(%s/%s) create launcher failed, error: %v", job.Namespace, job.Name, err)
				logger.Warn(msg)
				updateStatus(job.GetJobStatus(), commonv1.JobFailed, trainingJobFailedReason, msg)
			}
		case commonv1.RestartPolicyNever:
			job.Status.ReplicaStatuses[commonv1.ReplicaType(kaiv1alpha1.ETReplicaTypeLauncher)].Failed = 1
			msg := fmt.Sprintf("job(%s/%s) has failed", job.Namespace, job.Name)
			reason := trainingJobFailedReason
			r.recorder.Event(job, corev1.EventTypeWarning, reason, msg)
			if !isEvicted(*job.GetJobStatus()) && job.Status.CompletionTime == nil {
				now := metav1.Now()
				job.Status.CompletionTime = &now
			}
			updateStatus(job.GetJobStatus(), commonv1.JobFailed, reason, msg)
		}
		return nil
	}

	if isPodSucceeded(launcher) {
		job.Status.ReplicaStatuses[commonv1.ReplicaType(kaiv1alpha1.ETReplicaTypeLauncher)].Succeeded = 1
		msg := fmt.Sprintf("job(%s/%s) successfully completed.", job.Namespace, job.Name)
		r.recorder.Event(job, corev1.EventTypeNormal, trainingJobSucceededReason, msg)
		if job.Status.CompletionTime == nil {
			now := metav1.Now()
			job.Status.CompletionTime = &now
		}
		updateJobConditions(job.GetJobStatus(), commonv1.JobSucceeded, trainingJobSucceededReason, msg)
		updatePhase(job.GetJobStatus(), commonv1.JobSucceeded)
	} else if isPodFailed(launcher) {
		job.Status.ReplicaStatuses[commonv1.ReplicaType(kaiv1alpha1.ETReplicaTypeLauncher)].Failed = 1
		msg := fmt.Sprintf("job(%s/%s) has failed", job.Namespace, job.Name)
		reason := launcher.Status.Reason
		if reason == "" {
			reason = trainingJobFailedReason
		}
		r.recorder.Event(job, corev1.EventTypeWarning, reason, msg)
		if reason == "Evicted" {
			reason = trainingJobEvict
		} else if !isEvicted(*job.GetJobStatus()) && job.Status.CompletionTime == nil {
			now := metav1.Now()
			job.Status.CompletionTime = &now
		}
		updateStatus(job.GetJobStatus(), commonv1.JobFailed, reason, msg)
	} else if isPodRunning(launcher) {
		job.Status.ReplicaStatuses[commonv1.ReplicaType(kaiv1alpha1.ETReplicaTypeLauncher)].Active = 1
		if !isRunning(*job.GetJobStatus()) {
			msg := fmt.Sprintf("job(%s/%s) is running.", job.Namespace, job.Name)
			updateStatus(job.GetJobStatus(), commonv1.JobRunning, trainingJobRunningReason, msg)
			r.recorder.Event(job, corev1.EventTypeNormal, trainingJobRunningReason, msg)
		}
	}
	return nil
}

func (r *TrainingJobReconciler) executeOnLauncher(trainingJob *kaiv1alpha1.TrainingJob, script string) (string, string, error) {
	var err error
	var launcherPod *corev1.Pod
	if launcherPod, err = r.GetLauncherJob(trainingJob); err != nil {
		if errors.IsNotFound(err) {
			return "", "", fmt.Errorf("trainingjob(%s/%s) not found launcher", trainingJob.Namespace, trainingJob.Name)
		} else {
			return "", "", err
		}
	}

	if launcherPod != nil {
		logger.Infof("launcher %s execute script %s", launcherPod.Name, script)
		stdOut, stdErr, err := kubectlOnPod(launcherPod, script)
		if err != nil {
			// TODO: add backoff limit
			// update scaleout and trainingjob status
			return "", "", fmt.Errorf("execute script failed: %v", err)
		}
		return stdOut, stdErr, nil
	}
	return "", "", nil
}

func (r *TrainingJobReconciler) GetLauncherJob(obj interface{}) (*corev1.Pod, error) {
	job, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	launcher := &corev1.Pod{}
	name := ctrl.Request{}
	name.NamespacedName.Namespace = job.GetNamespace()
	name.NamespacedName.Name = job.GetName() + launcherSuffix
	err = r.Get(context.Background(), name.NamespacedName, launcher)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	return launcher, nil
}

func (r *TrainingJobReconciler) CreateLauncher(obj interface{}) (*corev1.Pod, error) {
	job, ok := obj.(*kaiv1alpha1.TrainingJob)
	if !ok {
		return nil, fmt.Errorf("%+v is not a type of TrainingJob", job)
	}
	launcher := newLauncher(job)
	if job.GetAttachMode() == kaiv1alpha1.AttachModeSSH {
		util.MountRsaKey(launcher, job.Name)
	}
	err := r.Create(context.Background(), launcher)
	if err != nil {
		r.recorder.Eventf(job, corev1.EventTypeWarning, trainingJobFailedReason, "launcher pod created failed: %v", err)
		return nil, err
	}
	logger.Infof("Success trainingjob(%v/%v) CreateLauncher", job.Namespace, job.Name)
	return launcher, nil
}

func (r *TrainingJobReconciler) GetOrCreateLauncherServiceAccount(obj interface{}) (*corev1.ServiceAccount, error) {
	job, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	sa := &corev1.ServiceAccount{}
	name := ctrl.Request{}
	name.NamespacedName.Namespace = job.GetNamespace()
	name.NamespacedName.Name = job.GetName() + launcherSuffix
	err = r.Get(context.Background(), name.NamespacedName, sa)
	if errors.IsNotFound(err) {
		logger.Infof("will create serviceAccount: %v", name.NamespacedName)
		r.Create(context.Background(), newLauncherServiceAccount(obj))
	}
	return sa, nil

}

func (r *TrainingJobReconciler) GetOrCreateLauncherRole(obj interface{}, workerReplicas int32) (*rbacv1.Role, error) {
	job, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	role := &rbacv1.Role{}
	name := ctrl.Request{}
	name.NamespacedName.Namespace = job.GetNamespace()
	name.NamespacedName.Name = job.GetName() + launcherSuffix
	err = r.Get(context.Background(), name.NamespacedName, role)
	if errors.IsNotFound(err) {
		logger.Infof("will create launcherRole: %v", name.NamespacedName)
		r.Create(context.Background(), newLauncherRole(obj, workerReplicas))
	}
	return role, nil
}

func (r *TrainingJobReconciler) GetLauncherRoleBinding(obj interface{}) (*rbacv1.RoleBinding, error) {
	job, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	rb := &rbacv1.RoleBinding{}
	name := ctrl.Request{}
	name.NamespacedName.Namespace = job.GetNamespace()
	name.NamespacedName.Name = job.GetName() + launcherSuffix
	err = r.Get(context.Background(), name.NamespacedName, rb)
	if errors.IsNotFound(err) {
		logger.Infof("will create launcherRoleBinding: %v", name.NamespacedName)
		r.Create(context.Background(), newLauncherRoleBinding(obj))
	}
	return rb, nil
}

func (r *TrainingJobReconciler) DeleteLauncher(job *kaiv1alpha1.TrainingJob) error {
	// delete pod
	if err := r.deleteLauncherPod(job); err != nil {
		logger.Errorf("trainingjob(%v/%v) fail to deleteLauncherPod", job.Namespace, job.Name)
		return err
	}
	logger.Infof("Success trainingjob(%v/%v) DeleteLauncher", job.Namespace, job.Name)

	// todo:delete sa
	// todo:delete role
	// todo: delete cm
	return nil
}
func (r *TrainingJobReconciler) deleteLauncherPod(job *kaiv1alpha1.TrainingJob) error {
	launcher, err := r.GetLauncherJob(job)
	if err != nil {
		logger.Errorf("trainingjob(%v/%v) fail to GetLauncherJob", job.Namespace, job.Name)
		return err
	}
	if launcher == nil {
		logger.Infof("trainingjob(%v/%v) launcher not found", job.Namespace, job.Name)
		return nil
	}
	deleteOptions := &client.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(0)}
	if err := r.Delete(context.Background(), launcher, deleteOptions); err != nil && !errors.IsNotFound(err) {
		r.recorder.Eventf(job, corev1.EventTypeWarning, trainingJobFailedReason, "Error deleting pod %s: %v", launcher.Name, err)
		//return err
	}
	r.recorder.Eventf(job, corev1.EventTypeNormal, trainingJobSucceededReason, "Deleted pod %s", launcher.Name)
	return nil
}

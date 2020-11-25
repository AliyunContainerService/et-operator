package controllers

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

func IsRequeueError(err error) bool {
	switch err.(type) {
	case RequeueError:
		return true
	}
	return false
}

type RequeueError struct {
	error
}

func NewRequeueError(err error) RequeueError {
	return RequeueError{err}
}

// NoRequeue does not requeue when Requeue is False and duration is 0.
func NoRequeue() (ctrl.Result, error) {
	return RequeueIfError(nil)
}

// RequeueIfError requeues if an error is found.
func RequeueIfError(err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}

// RequeueImmediately requeues immediately when Requeue is True and no duration is specified.
func RequeueImmediately() (ctrl.Result, error) {
	return ctrl.Result{Requeue: true}, nil
}

// RequeueAfterInterval requeues after a duration when duration > 0 is specified.
func RequeueAfterInterval(interval time.Duration, err error) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: interval}, err
}

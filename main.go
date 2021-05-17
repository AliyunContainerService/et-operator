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

package main

import (
	"flag"
	"os"
	"time"

	kaiv1alpha1 "github.com/AliyunContainerService/et-operator/api/v1alpha1"
	"github.com/AliyunContainerService/et-operator/pkg/controllers"
	zapOpt "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
	"go.uber.org/zap/zapcore"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = kaiv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	//ctrl.SetLogger(zap.New(func(o *zap.Options) {
	//	o.Development = true
	//}))

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = false
	}, func(o *zap.Options) {
		o.ZapOpts = append(o.ZapOpts, zapOpt.AddCaller())
	}, func(o *zap.Options) {
		encCfg := zapOpt.NewProductionEncoderConfig()
		encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		o.Encoder = zapcore.NewConsoleEncoder(encCfg)
	}))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	const jobPollInterval = "5s"
	if err = controllers.NewReconciler(mgr, parseDurationOrPanic(jobPollInterval)).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TrainingJob")
		os.Exit(1)
	}
	if err = controllers.NewScaleOutReconciler(mgr, parseDurationOrPanic(jobPollInterval)).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScaleOut")
		os.Exit(1)
	}
	if err = controllers.NewScaleInReconciler(mgr, parseDurationOrPanic(jobPollInterval)).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScaleIn")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting et-operator manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parseDurationOrPanic(duration string) time.Duration {
	if parsed, err := time.ParseDuration(duration); err != nil {
		panic("Unable to parse duration: " + duration)
	} else {
		return parsed
	}
}

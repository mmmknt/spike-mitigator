/*
Copyright 2020 mmmknt.

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

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	istiocli "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/mmmknt/spike-mitigator/api/v1alpha1"
	"github.com/mmmknt/spike-mitigator/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(api.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "ab363dc0.mmmknt.dev",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		setupLog.Error(err, "unable to get InClusterConfig")
		os.Exit(1)
	}
	istioClientset, err := istiocli.NewForConfig(restConfig)
	if err != nil {
		setupLog.Error(err, "unable to create istio Clientset")
		os.Exit(1)
	}
	kubeClientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes Clientset")
		os.Exit(1)
	}
	ddConfig := datadog.NewConfiguration()
	ddClient := datadog.NewAPIClient(ddConfig)
	factory := informers.NewSharedInformerFactory(kubeClientset, time.Minute*5)
	sl := factory.Core().V1().Secrets().Lister()
	calculator := &controllers.MitigationCalculator{
		KubernetesClientset: kubeClientset,
		DDClient:            ddClient,
		SecretLister:        sl,
	}
	stopCh := ctrl.SetupSignalHandler()
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	if err = (&controllers.BalancingRuleReconciler{
		Client:         mgr.GetClient(),
		IstioClientset: istioClientset,
		Calculator:     calculator,
		Log:            ctrl.Log.WithName("controllers").WithName("MitigationRule"),
		Scheme:         mgr.GetScheme(),
		SecretLister:   sl,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MitigationRule")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

/*
Copyright 2022.

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

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/controllers/nodekeeper"
	"github.com/openshift/managed-upgrade-operator/controllers/upgradeconfig"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"github.com/openshift/managed-upgrade-operator/pkg/eventmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scheduler"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	cub "github.com/openshift/managed-upgrade-operator/pkg/upgraders"
	"github.com/openshift/managed-upgrade-operator/pkg/validation"
	"github.com/openshift/managed-upgrade-operator/util"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(upgradev1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	operatorNS, err := util.GetOperatorNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine operator namespace, please define OPERATOR_NAMESPACE")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Namespace:              operatorNS,
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "312e6264.managed.openshift.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create a separate client for the OAH Builder
	kubeConfig := ctrl.GetConfigOrDie()
	handlerClient, err := client.New(kubeConfig, client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		os.Exit(1)
	}

	// Add UpgradeConfig controller to the manager
	if err = (&upgradeconfig.ReconcileUpgradeConfig{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		MetricsClientBuilder:   metrics.NewBuilder(handlerClient),
		ClusterUpgraderBuilder: cub.NewBuilder(handlerClient),
		ValidationBuilder:      validation.NewBuilder(handlerClient),
		ConfigManagerBuilder:   configmanager.NewBuilder(handlerClient),
		Scheduler:              scheduler.NewScheduler(),
		CvClientBuilder:        cv.NewBuilder(handlerClient),
		EventManagerBuilder:    eventmanager.NewBuilder(handlerClient),
		UcMgrBuilder:           ucm.NewBuilder(handlerClient),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UpgradeConfig")
		os.Exit(1)
	}

	// Add NodeKeeper controller to the manager
	if err = (&nodekeeper.ReconcileNodeKeeper{
		Client:                      mgr.GetClient(),
		Scheme:                      mgr.GetScheme(),
		ConfigManagerBuilder:        configmanager.NewBuilder(handlerClient),
		Machinery:                   machinery.NewMachinery(),
		MetricsClientBuilder:        metrics.NewBuilder(handlerClient),
		DrainstrategyBuilder:        drain.NewBuilder(handlerClient),
		UpgradeConfigManagerBuilder: upgradeconfigmanager.NewBuilder(handlerClient),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodeKeeper")
		os.Exit(1)
	}

	// Add MachineConfigPool controller to the manager
	if err = (&nodekeeper.ReconcileNodeKeeper{
		Client:                      mgr.GetClient(),
		Scheme:                      mgr.GetScheme(),
		UpgradeConfigManagerBuilder: ucm.NewBuilder(handlerClient),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineConfigPool")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	zaplogfmt "github.com/sykesm/zap-logfmt"
	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	muocfg "github.com/openshift/managed-upgrade-operator/config"
	"github.com/openshift/managed-upgrade-operator/controllers/machineconfigpool"
	"github.com/openshift/managed-upgrade-operator/controllers/nodekeeper"
	"github.com/openshift/managed-upgrade-operator/controllers/upgradeconfig"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/collector"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"github.com/openshift/managed-upgrade-operator/pkg/eventmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/k8sutil"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scheduler"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	cub "github.com/openshift/managed-upgrade-operator/pkg/upgraders"
	"github.com/openshift/managed-upgrade-operator/pkg/validation"
	"github.com/openshift/managed-upgrade-operator/util"
	"github.com/openshift/managed-upgrade-operator/version"

	opmetrics "github.com/openshift/operator-custom-metrics/pkg/metrics"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	monclientv1 "github.com/coreos/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"
	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"

	routev1 "github.com/openshift/api/route/v1"

	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	//+kubebuilder:scaffold:imports
)

var (
	metricsHost             = "0.0.0.0"
	metricsPort       int32 = 8383
	customMetricsPath       = "/metrics"
	scheme                  = apiruntime.NewScheme()
	setupLog                = ctrl.Log.WithName("setup")
)
var log = logf.Log.WithName("cmd")

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(machineconfigapi.AddToScheme(scheme))
	utilruntime.Must(upgradev1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(machineapi.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func printVersion() {
	log.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: v%v", version.SDKVersion))
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

	// Add a custom logger to log in RFC3339 format instead of UTC
	configLog := uzap.NewProductionEncoderConfig()
	configLog.EncodeTime = func(ts time.Time, encoder zapcore.PrimitiveArrayEncoder) {
		encoder.AppendString(ts.UTC().Format(time.RFC3339Nano))
	}
	logfmtEncoder := zaplogfmt.NewEncoder(configLog)
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout), zap.Encoder(logfmtEncoder))
	logf.SetLogger(logger)
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	printVersion()

	operatorNS, err := k8sutil.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine operator namespace, please define OPERATOR_NAMESPACE")
		os.Exit(1)
	}

	if err := monitoringv1.AddToScheme(clientgoscheme.Scheme); err != nil {
		setupLog.Error(err, "unable to add monitoringv1 scheme")
		os.Exit(1)
	}

	// This set the sync period to 5m
	syncPeriod := time.Duration(muocfg.SyncPeriodDefault)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Namespace:              operatorNS,
		Scheme:                 scheme,
		MetricsBindAddress:     fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "312e6264.managed.openshift.io",
		SyncPeriod:             &syncPeriod,
		NewClient: func(_ cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {
			return client.New(config, options)
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Add UpgradeConfig controller to the manager
	if err = (&upgradeconfig.ReconcileUpgradeConfig{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		MetricsClientBuilder:   metrics.NewBuilder(),
		ClusterUpgraderBuilder: cub.NewBuilder(),
		ValidationBuilder:      validation.NewBuilder(),
		ConfigManagerBuilder:   configmanager.NewBuilder(),
		Scheduler:              scheduler.NewScheduler(),
		CvClientBuilder:        cv.NewBuilder(),
		EventManagerBuilder:    eventmanager.NewBuilder(),
		UcMgrBuilder:           ucm.NewBuilder(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UpgradeConfig")
		os.Exit(1)
	}

	// Add NodeKeeper controller to the manager
	if err = (&nodekeeper.ReconcileNodeKeeper{
		Client:                      mgr.GetClient(),
		Scheme:                      mgr.GetScheme(),
		ConfigManagerBuilder:        configmanager.NewBuilder(),
		Machinery:                   machinery.NewMachinery(),
		MetricsClientBuilder:        metrics.NewBuilder(),
		DrainstrategyBuilder:        drain.NewBuilder(),
		UpgradeConfigManagerBuilder: upgradeconfigmanager.NewBuilder(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodeKeeper")
		os.Exit(1)
	}

	// Add MachineConfigPool controller to the manager
	if err = (&machineconfigpool.ReconcileMachineConfigPool{
		Client:                      mgr.GetClient(),
		Scheme:                      mgr.GetScheme(),
		UpgradeConfigManagerBuilder: ucm.NewBuilder(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineConfigPool")
		os.Exit(1)
	}

	ctx := context.TODO()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Add the Metrics Service and ServiceMonitor
	if err := addMetrics(ctx, cfg); err != nil {
		log.Error(err, "Metrics service is not added.")
		os.Exit(1)
	}

	// Add the Custom Metrics Service
	metricsClient, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "unable to create k8s client for upgrade metrics")
		os.Exit(1)
	}

	uCollector, err := collector.NewUpgradeCollector(metricsClient)
	if err != nil {
		log.Error(err, "unable to create upgrade metrics collector")
		os.Exit(1)
	}

	ns, err := util.GetOperatorNamespace()
	if err != nil {
		os.Exit(1)
	}

	customMetrics := opmetrics.NewBuilder(ns, "managed-upgrade-operator-custom-metrics").
		WithPath(customMetricsPath).
		WithCollector(uCollector).
		WithServiceMonitor().
		WithServiceLabel(map[string]string{"name": muocfg.OperatorName}).
		GetConfig()

	if err = opmetrics.ConfigureMetrics(context.TODO(), *customMetrics); err != nil {
		log.Error(err, "Failed to configure custom metrics")
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

	// Define stopCh which we'll use to notify the upgradeConfigManager (and any other routine)
	// to stop work. This channel can also be used to signal routines to complete any cleanup
	// work
	stopCh := signals.SetupSignalHandler()

	upgradeConfigManagerClient, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "unable to create configmanager client")
		os.Exit(1)
	}

	ucMgr, err := upgradeconfigmanager.NewBuilder().NewManager(upgradeConfigManagerClient)
	if err != nil {
		log.Error(err, "can't read config manager configuration")
	}
	log.Info("Starting UpgradeConfig manager")
	go ucMgr.StartSync(stopCh)

	setupLog.Info("starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// addMetrics will create the Services and Service Monitors to allow the operator export the metrics by using
// the Prometheus operator
func addMetrics(ctx context.Context, cfg *rest.Config) error {
	// Get the namespace the operator is currently deployed in.
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		if errors.Is(err, k8sutil.ErrRunLocal) || errors.Is(err, k8sutil.ErrNoNamespace) {
			log.Info("Skipping metrics service creation; not running in a cluster.")
			return nil
		}
	}

	// Get the operator name
	operatorName, _ := k8sutil.GetOperatorName()

	service, err := opmetrics.GenerateService(metricsPort, "http-metrics", operatorName+"-metrics", operatorNs, map[string]string{"name": operatorName})
	if err != nil {
		log.Info("Could not create metrics Service", "error", err.Error())
	}

	services := []*corev1.Service{service}
	mclient := monclientv1.NewForConfigOrDie(cfg)
	copts := metav1.CreateOptions{}

	for _, s := range services {
		if s == nil {
			continue
		}

		sm := opmetrics.GenerateServiceMonitor(s)

		// ErrSMMetricsExists is used to detect if the -metrics ServiceMonitor already exists
		var ErrSMMetricsExists = fmt.Sprintf("servicemonitors.monitoring.coreos.com \"%s-metrics\" already exists", muocfg.OperatorName)

		log.Info(fmt.Sprintf("Attempting to create service monitor %s", sm.Name))
		// TODO: Get SM and compare to see if an UPDATE is required
		_, err := mclient.ServiceMonitors(operatorNs).Create(ctx, sm, copts)
		if err != nil {
			if err.Error() != ErrSMMetricsExists {
				return err
			}
			log.Info("ServiceMonitor already exists")
		}
		log.Info(fmt.Sprintf("Successfully configured service monitor %s", sm.Name))
	}
	return nil
}

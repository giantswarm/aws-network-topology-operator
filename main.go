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
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap/zapcore"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/aws/aws-sdk-go/aws/session"
	gocache "github.com/patrickmn/go-cache"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/giantswarm/aws-network-topology-operator/controllers"
	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/registrar"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(capi.AddToScheme(scheme))
	utilruntime.Must(capa.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var managementClusterName string
	var managementClusterNamespace string
	var snsTopic string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&managementClusterName, "management-cluster-name", "", "The name of the Cluster CR for the management cluster")
	flag.StringVar(&managementClusterNamespace, "management-cluster-namespace", "", "The namespace of the Cluster CR for the management cluster")
	flag.StringVar(&snsTopic, "sns-topic", "", "The SNS topic to send TGW attatchment requests to when running in UserManaged mode")
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if managementClusterName == "" || managementClusterNamespace == "" {
		setupLog.Error(fmt.Errorf("management-cluster-name and management-cluster-namespace required"), "Management Cluster details required")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "bd3aa545.giantswarm.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := context.TODO()

	managementCluster := types.NamespacedName{
		Name:      managementClusterName,
		Namespace: managementClusterNamespace,
	}
	client := k8sclient.NewCluster(mgr.GetClient(), managementCluster)
	session := session.Must(session.NewSession())

	ec2Service := aws.NewEC2Client(ctx, client, managementCluster)
	snsService := aws.NewSNSClient(ctx, snsTopic, client, managementCluster)

	identity, err := client.GetAWSClusterRoleIdentity(ctx, managementCluster)
	if err != nil {
		setupLog.Error(err, "unable to get management cluster's AWS Cluster Role Identity")
		os.Exit(1)
	}

	ramService := aws.NewRAMClient(aws.AwsRamClientFromClusterRoleIdentity(session, identity.Spec.RoleArn, identity.Spec.ExternalID))

	// Cache EC2 clients to avoid lots of credential requests due to client recreation
	expiration := 5 * time.Minute
	transitGatewayClientForWorkloadClusterEC2ClientCache := gocache.New(expiration, expiration/2)
	getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) aws.TransitGatewayClient {
		v, ok := transitGatewayClientForWorkloadClusterEC2ClientCache.Get(workloadCluster.String())
		var ec2ServiceWorkloadCluster *aws.EC2Client
		if ok {
			ec2ServiceWorkloadCluster = v.(*aws.EC2Client)
		} else {
			ec2ServiceWorkloadCluster = aws.NewEC2Client(
				ctx,

				// k8s client points to management cluster since that has the `AWSCluster` object which
				// in turn references `AWSClusterRoleIdentity` to determine the AWS account of the
				// workload cluster
				client,

				workloadCluster)

			transitGatewayClientForWorkloadClusterEC2ClientCache.SetDefault(workloadCluster.String(), ec2ServiceWorkloadCluster)
		}

		return aws.NewTGWClient(*ec2ServiceWorkloadCluster, *snsService)
	}

	registrars := []controllers.Registrar{
		registrar.NewTransitGateway(aws.NewTGWClient(*ec2Service, *snsService), client, getTransitGatewayClientForWorkloadCluster),
	}
	controller := controllers.NewNetworkTopologyReconciler(client, registrars)
	err = controller.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "failed to setup controller", "controller", "Cluster")
		os.Exit(1)
	}
	shareController := controllers.NewShareReconciler(client, ramService)
	err = shareController.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "failed to setup controller", "controller", "Share")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

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

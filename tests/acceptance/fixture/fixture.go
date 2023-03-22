package fixture

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ram"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gsannotations "github.com/giantswarm/k8smetadata/pkg/annotation"

	"github.com/giantswarm/aws-network-topology-operator/controllers"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/registrar"
)

const (
	ClusterVCPCIDR    = "172.64.0.0/16"
	ClusterSubnetCIDR = "172.64.0.0/20"
)

func NewFixture(k8sClient client.Client, config Config) *Fixture {
	session, err := session.NewSession(&awssdk.Config{
		Region: awssdk.String(config.AWSRegion),
	})
	Expect(err).NotTo(HaveOccurred())

	ec2Client := ec2.New(session,
		&awssdk.Config{
			Credentials: stscreds.NewCredentials(session, config.AWSIAMRoleARN),
		},
	)

	ramClient := ram.New(session, &awssdk.Config{
		Credentials: stscreds.NewCredentials(session, config.AWSIAMRoleARN),
	})

	return &Fixture{
		K8sClient: k8sClient,

		EC2Client: ec2Client,
		RamClient: ramClient,
		config:    config,
	}
}

type Network struct {
	AssociationID string
	RouteTableID  string
	SubnetID      string
	VpcID         string
}

type Cluster struct {
	Cluster             *capi.Cluster
	AWSCluster          *capa.AWSCluster
	ClusterRoleIdentity *capa.AWSClusterRoleIdentity
}

type Config struct {
	AWSAccount                 string
	AWSIAMRoleARN              string
	AWSRegion                  string
	ManagementClusterName      string
	ManagementClusterNamespace string
}

type Fixture struct {
	K8sClient client.Client
	EC2Client *ec2.EC2
	RamClient *ram.RAM

	Network           Network
	ManagementCluster Cluster

	config Config
}

func (f *Fixture) Setup() error {
	f.Network = f.createNetwork()
	f.ManagementCluster = f.createCluster(f.Network)

	return nil
}

func (f *Fixture) Teardown() error {
	f.deleteCluster()

	err := DeleteKubernetesObject(f.K8sClient, f.ManagementCluster.AWSCluster)
	Expect(err).NotTo(HaveOccurred())

	err = DeleteKubernetesObject(f.K8sClient, f.ManagementCluster.ClusterRoleIdentity)
	Expect(err).NotTo(HaveOccurred())

	err = DisassociateRouteTable(f.EC2Client, f.Network.AssociationID)
	Expect(err).NotTo(HaveOccurred())

	err = DeleteRouteTable(f.EC2Client, f.Network.RouteTableID)
	Expect(err).NotTo(HaveOccurred())

	err = DeleteSubnet(f.EC2Client, f.Network.SubnetID)
	Expect(err).NotTo(HaveOccurred())

	err = DeleteVPC(f.EC2Client, f.Network.VpcID)
	Expect(err).NotTo(HaveOccurred())

	return nil
}

func (f *Fixture) createNetwork() Network {
	createVpcOutput, err := f.EC2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: awssdk.String(ClusterVCPCIDR),
	})
	Expect(err).NotTo(HaveOccurred())

	vpcID := *createVpcOutput.Vpc.VpcId

	createSubnetOutput, err := f.EC2Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock:         awssdk.String(ClusterSubnetCIDR),
		VpcId:             awssdk.String(vpcID),
		AvailabilityZone:  awssdk.String(getAvailabilityZone(f.config.AWSRegion)),
		TagSpecifications: generateTagSpecifications(f.config.ManagementClusterName),
	})
	Expect(err).NotTo(HaveOccurred())
	subnetID := *createSubnetOutput.Subnet.SubnetId

	createRouteTableOutput, err := f.EC2Client.CreateRouteTable(&ec2.CreateRouteTableInput{
		VpcId: awssdk.String(vpcID),
	})
	Expect(err).NotTo(HaveOccurred())

	routeTableID := *createRouteTableOutput.RouteTable.RouteTableId

	assocRouteTableOutput, err := f.EC2Client.AssociateRouteTable(&ec2.AssociateRouteTableInput{
		RouteTableId: awssdk.String(routeTableID),
		SubnetId:     awssdk.String(subnetID),
	})
	Expect(err).NotTo(HaveOccurred())

	associationID := *assocRouteTableOutput.AssociationId

	return Network{
		VpcID:         vpcID,
		SubnetID:      subnetID,
		AssociationID: associationID,
		RouteTableID:  routeTableID,
	}
}

func (f *Fixture) createCluster(network Network) Cluster {
	ctx := context.Background()

	clusterRoleIdentity := &capa.AWSClusterRoleIdentity{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.config.ManagementClusterName,
		},
		Spec: capa.AWSClusterRoleIdentitySpec{
			AWSRoleSpec: capa.AWSRoleSpec{
				RoleArn: f.config.AWSIAMRoleARN,
			},
			SourceIdentityRef: &capa.AWSIdentityReference{
				Name: "default",
				Kind: capa.ControllerIdentityKind,
			},
		},
	}

	err := f.K8sClient.Create(ctx, clusterRoleIdentity)
	Expect(err).NotTo(HaveOccurred())

	cluster := &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.config.ManagementClusterName,
			Namespace: "test",
			Annotations: map[string]string{
				gsannotations.NetworkTopologyModeAnnotation: gsannotations.NetworkTopologyModeGiantSwarmManaged,
			},
		},
		Spec: capi.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: capa.GroupVersion.String(),
				Kind:       "AWSCluster",
				Namespace:  "test",
				Name:       f.config.ManagementClusterName,
			},
		},
	}

	err = f.K8sClient.Create(ctx, cluster)
	Expect(err).NotTo(HaveOccurred())

	awsCluster := &capa.AWSCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.config.ManagementClusterName,
			Namespace: "test",
		},
		Spec: capa.AWSClusterSpec{
			IdentityRef: &capa.AWSIdentityReference{
				Name: f.config.ManagementClusterName,
				Kind: capa.ClusterRoleIdentityKind,
			},
			NetworkSpec: capa.NetworkSpec{
				VPC: capa.VPCSpec{
					ID:        network.VpcID,
					CidrBlock: ClusterVCPCIDR,
				},
				Subnets: []capa.SubnetSpec{
					{
						CidrBlock: ClusterSubnetCIDR,
						ID:        network.SubnetID,
						IsPublic:  false,
					},
				},
			},
		},
	}

	err = f.K8sClient.Create(ctx, awsCluster)
	Expect(err).NotTo(HaveOccurred())

	return Cluster{
		Cluster:             cluster,
		AWSCluster:          awsCluster,
		ClusterRoleIdentity: clusterRoleIdentity,
	}
}

func (f *Fixture) deleteCluster() {
	cluster := f.ManagementCluster.Cluster
	err := DeleteKubernetesObject(f.K8sClient, cluster)
	Expect(err).NotTo(HaveOccurred())

	if cluster == nil {
		return
	}
	Eventually(func() []string {
		actualCluster := &capi.Cluster{}
		err := f.K8sClient.Get(context.Background(), k8sclient.GetNamespacedName(cluster), actualCluster)
		if k8serrors.IsNotFound(err) {
			return []string{}
		}
		return actualCluster.Finalizers
	}).ShouldNot(ContainElement(controllers.FinalizerNetTop))
}

func getAvailabilityZone(region string) string {
	return fmt.Sprintf("%sa", region)
}

func generateTagSpecifications(name string) []*ec2.TagSpecification {
	tagSpec := &ec2.TagSpecification{
		ResourceType: awssdk.String(ec2.ResourceTypeSubnet),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String(registrar.SubnetTGWAttachementsLabel),
				Value: awssdk.String("true"),
			},
			{
				Key:   awssdk.String(registrar.SubnetRoleLabel),
				Value: awssdk.String("private"),
			},
			{
				Key:   awssdk.String(capa.NameKubernetesAWSCloudProviderPrefix + name),
				Value: awssdk.String("shared"),
			},
		},
	}

	tagSpecifications := make([]*ec2.TagSpecification, 0)
	tagSpecifications = append(tagSpecifications, tagSpec)
	return tagSpecifications
}

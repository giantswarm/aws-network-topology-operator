package acceptance

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ram"
	gsannotations "github.com/giantswarm/k8smetadata/pkg/annotation"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/registrar"
	"github.com/giantswarm/aws-network-topology-operator/tests"
)

func NewFixture(k8sClient client.Client) *Fixture {
	mcAccount := tests.GetEnvOrSkip("MC_AWS_ACCOUNT")
	wcAccount := tests.GetEnvOrSkip("WC_AWS_ACCOUNT")
	iamRoleId := tests.GetEnvOrSkip("AWS_IAM_ROLE_ID")
	awsRegion := tests.GetEnvOrSkip("AWS_REGION")
	managementClusterName := tests.GetEnvOrSkip("MANAGEMENT_CLUSTER_NAME")

	session, err := session.NewSession(&awssdk.Config{
		Region: awssdk.String(awsRegion),
	})
	Expect(err).NotTo(HaveOccurred())

	mcIAMRoleARN := getRoleARN(mcAccount, iamRoleId)
	wcIAMRoleARN := getRoleARN(wcAccount, iamRoleId)

	mcEC2Client := ec2.New(session,
		&awssdk.Config{
			Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
		},
	)

	wcEC2Client := ec2.New(session,
		&awssdk.Config{
			Credentials: stscreds.NewCredentials(session, wcIAMRoleARN),
		},
	)

	ramClient := ram.New(session, &awssdk.Config{
		Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
	})

	return &Fixture{
		K8sClient: k8sClient,

		McEC2Client: mcEC2Client,
		WcEC2Client: wcEC2Client,
		RamClient:   ramClient,

		config: &Config{
			ManagementAWSAccount:  mcAccount,
			WorkloadAWSAccount:    wcAccount,
			AwsIAMRoleId:          iamRoleId,
			AwsRegion:             awsRegion,
			ManagementClusterName: managementClusterName,
			ManagementClusterCIDR: "172.64.0.0",
			WorkloadClusterCIDR:   "172.96.0.0",
		},
	}
}

type Cluster struct {
	cluster             *capi.Cluster
	awsCluster          *capa.AWSCluster
	clusterRoleIdentity *capa.AWSClusterRoleIdentity

	AssocitationID string
	RouteTableId   string
	SubnetId       string
	VpcId          string
}

type Config struct {
	ManagementAWSAccount  string
	WorkloadAWSAccount    string
	AwsIAMRoleId          string
	AwsRegion             string
	ManagementClusterName string
	WorkloadClusterCIDR   string
	ManagementClusterCIDR string
}

type Fixture struct {
	K8sClient   client.Client
	McEC2Client *ec2.EC2
	WcEC2Client *ec2.EC2
	RamClient   *ram.RAM

	ManagementCluster *Cluster
	WorkloadClusters  []*Cluster

	config *Config
}

func (f *Fixture) Setup() error {
	cluster := &Cluster{}
	if err := cluster.Setup(f.config, f.K8sClient, f.McEC2Client, f.config.ManagementClusterName, f.config.ManagementClusterCIDR, f.config.ManagementAWSAccount); err != nil {
		return err
	}
	f.ManagementCluster = cluster
	return nil
}

func (f *Fixture) Teardown() error {
	if err := f.ManagementCluster.Teardown(f.K8sClient, f.McEC2Client); err != nil {
		return err
	}

	for _, cluster := range f.WorkloadClusters {
		if err := cluster.Teardown(f.K8sClient, f.McEC2Client); err != nil {
			return err
		}
	}
	return nil
}

func (f *Fixture) CreateWorkloadCluster() (*Cluster, error) {
	workloadCluster := &Cluster{}
	name := tests.GenerateGUID("test-wc")
	if err := workloadCluster.Setup(f.config, f.K8sClient, f.WcEC2Client, name, f.config.WorkloadClusterCIDR, f.config.WorkloadAWSAccount); err != nil {
		return nil, err
	}
	f.WorkloadClusters = append(f.WorkloadClusters, workloadCluster)
	return workloadCluster, nil
}

func (c *Cluster) GetClusterNamespacedName() types.NamespacedName {
	return k8sclient.GetNamespacedName(c.cluster)
}

func (c *Cluster) GetCluster() *capi.Cluster {
	return c.cluster
}

func (c *Cluster) Name() string {
	return c.cluster.Name
}

func (c *Cluster) GetAWSClusterNamespacedName() types.NamespacedName {
	return k8sclient.GetNamespacedName(c.awsCluster)
}

func (c *Cluster) GetAWSCluster() *capa.AWSCluster {
	return c.awsCluster
}

func (c *Cluster) GetClusterRoleIdentity() *capa.AWSClusterRoleIdentity {
	return c.clusterRoleIdentity
}

func (c *Cluster) Setup(config *Config, k8sClient client.Client, ec2Client *ec2.EC2, name string, CIDR string, awsAccount string) error {
	ctx := context.Background()
	vpcCIDR := fmt.Sprintf("%s/%d", CIDR, 16)
	subnetCIDR := fmt.Sprintf("%s/%d", CIDR, 20)

	createVpcOutput, err := ec2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: awssdk.String(vpcCIDR),
	})
	if err != nil {
		return fmt.Errorf("error while creating vpc: %w", err)
	}

	c.VpcId = *createVpcOutput.Vpc.VpcId

	createSubnetOutput, err := ec2Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock:         awssdk.String(subnetCIDR),
		VpcId:             awssdk.String(c.VpcId),
		AvailabilityZone:  awssdk.String(getAvailabilityZone(config.AwsRegion)),
		TagSpecifications: generateTagSpecifications(name),
	})
	if err != nil {
		return fmt.Errorf("error while creating subnet: %w", err)
	}
	c.SubnetId = *createSubnetOutput.Subnet.SubnetId

	createRouteTableOutput, err := ec2Client.CreateRouteTable(&ec2.CreateRouteTableInput{
		VpcId: awssdk.String(c.VpcId),
	})
	if err != nil {
		return fmt.Errorf("error while creating route table: %w", err)
	}
	c.RouteTableId = *createRouteTableOutput.RouteTable.RouteTableId

	assocRouteTableOutput, err := ec2Client.AssociateRouteTable(&ec2.AssociateRouteTableInput{
		RouteTableId: awssdk.String(c.RouteTableId),
		SubnetId:     awssdk.String(c.SubnetId),
	})
	if err != nil {
		return fmt.Errorf("error while associating route table with subnet: %w", err)
	}

	c.AssocitationID = *assocRouteTableOutput.AssociationId

	c.clusterRoleIdentity = &capa.AWSClusterRoleIdentity{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: capa.AWSClusterRoleIdentitySpec{
			AWSRoleSpec: capa.AWSRoleSpec{
				RoleArn: getRoleARN(awsAccount, config.AwsIAMRoleId),
			},
			SourceIdentityRef: &capa.AWSIdentityReference{
				Name: "default",
				Kind: capa.ControllerIdentityKind,
			},
		},
	}

	err = k8sClient.Create(ctx, c.clusterRoleIdentity)
	if err != nil {
		return fmt.Errorf("error while creating AWSClusterRoleIdentity: %w", err)
	}

	c.cluster = &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
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
				Name:       name,
			},
		},
	}
	c.awsCluster = &capa.AWSCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
		},
		Spec: capa.AWSClusterSpec{
			IdentityRef: &capa.AWSIdentityReference{
				Name: name,
				Kind: capa.ClusterRoleIdentityKind,
			},
			NetworkSpec: capa.NetworkSpec{
				VPC: capa.VPCSpec{
					ID:        c.VpcId,
					CidrBlock: vpcCIDR,
				},
				Subnets: []capa.SubnetSpec{
					{
						CidrBlock: subnetCIDR,
						ID:        c.SubnetId,
						IsPublic:  false,
					},
				},
			},
		},
	}

	err = k8sClient.Create(ctx, c.cluster)
	if err != nil {
		return fmt.Errorf("error while creating Cluster: %w", err)
	}

	err = k8sClient.Create(ctx, c.awsCluster)
	if err != nil {
		return fmt.Errorf("error while creating AWSCluster: %w", err)
	}
	return nil
}

func (c *Cluster) Teardown(k8sClient client.Client, ec2Client *ec2.EC2) error {
	_, err := ec2Client.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
		AssociationId: &c.AssocitationID,
	})
	if err != nil {
		return fmt.Errorf("error while disassociating route table with subnet: %w", err)
	}

	_, err = ec2Client.DeleteRouteTable(&ec2.DeleteRouteTableInput{
		RouteTableId: &c.RouteTableId,
	})
	if err != nil {
		return fmt.Errorf("error while deleting route table : %w", err)
	}

	_, err = ec2Client.DeleteSubnet(&ec2.DeleteSubnetInput{
		SubnetId: awssdk.String(c.SubnetId),
	})
	if err != nil {
		return fmt.Errorf("error while MC deleting subnet: %w", err)
	}

	_, err = ec2Client.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: awssdk.String(c.VpcId),
	})
	if err != nil {
		return fmt.Errorf("error while WC deleting vpc: %w", err)
	}
	return nil
}

func getRoleARN(account, roleID string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", account, roleID)
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

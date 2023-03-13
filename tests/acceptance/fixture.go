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

	session, err := session.NewSession(&awssdk.Config{
		Region: awssdk.String(awsRegion),
	})
	Expect(err).NotTo(HaveOccurred())

	mcIAMRoleARN := getRoleARN(mcAccount, iamRoleId)

	ec2Client := ec2.New(session,
		&awssdk.Config{
			Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
		},
	)

	ramClient := ram.New(session, &awssdk.Config{
		Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
	})

	return &Fixture{
		k8sClient: k8sClient,

		ec2Client: ec2Client,
		ramClient: ramClient,

		managementAWSAccount: mcAccount,
		workloadAWSAccount:   wcAccount,
		awsIAMRoleId:         iamRoleId,
		awsRegion:            awsRegion,
	}
}

type Fixture struct {
	k8sClient client.Client
	ec2Client *ec2.EC2
	ramClient *ram.RAM

	managementAWSAccount string
	workloadAWSAccount   string
	awsIAMRoleId         string
	awsRegion            string

	managementCluster             *capi.Cluster
	managementAWSCluster          *capa.AWSCluster
	managementClusterRoleIdentity *capa.AWSClusterRoleIdentity

	workloadCluster             *capi.Cluster
	workloadAWSCluster          *capa.AWSCluster
	workloadClusterRoleIdentity *capa.AWSClusterRoleIdentity

	associtationID string
	routeTableId   string
	subnetId       string
	vpcId          string
	vpcIdWC        string
	subnetIdWC     string
}

func (f *Fixture) GetManagementClusterNamespacedName() types.NamespacedName {
	return k8sclient.GetNamespacedName(f.managementCluster)
}

func (f *Fixture) GetManagementCluster() *capi.Cluster {
	return f.managementCluster
}

func (f *Fixture) GetManagementAWSClusterNamespacedName() types.NamespacedName {
	return k8sclient.GetNamespacedName(f.managementAWSCluster)
}

func (f *Fixture) GetManagementAWSCluster() *capa.AWSCluster {
	return f.managementAWSCluster
}

func (f *Fixture) GetWorkloadClusterNamespacedName() types.NamespacedName {
	return k8sclient.GetNamespacedName(f.workloadCluster)
}

func (f *Fixture) GetWorkloadCluster() *capi.Cluster {
	return f.workloadCluster
}

func (f *Fixture) GetWorkloadAWSClusterNamespacedName() types.NamespacedName {
	return k8sclient.GetNamespacedName(f.workloadAWSCluster)
}

func (f *Fixture) GetWorkloadAWSCluster() *capa.AWSCluster {
	return f.workloadAWSCluster
}

func (f *Fixture) GetClusterRoleIdentity() *capa.AWSClusterRoleIdentity {
	return f.managementClusterRoleIdentity
}

func (f *Fixture) GetVpcID() string {
	return f.vpcId
}

func (f *Fixture) GetWorkloadClusterVpcID() string {
	return f.vpcIdWC
}

func (f *Fixture) GetWorkloadClusterRoleIdentity() *capa.AWSClusterRoleIdentity {
	return f.workloadClusterRoleIdentity
}

func (f *Fixture) Setup(ctx context.Context) error {
	name := tests.GenerateGUID("test")

	createVpcOutput, err := f.ec2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: awssdk.String("172.64.0.0/16"),
	})
	if err != nil {
		return fmt.Errorf("error while creating vpc: %w", err)
	}

	f.vpcId = *createVpcOutput.Vpc.VpcId

	createSubnetOutput, err := f.ec2Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock:         awssdk.String("172.64.0.0/20"),
		VpcId:             awssdk.String(f.vpcId),
		AvailabilityZone:  awssdk.String(getAvailabilityZone(f.awsRegion)),
		TagSpecifications: generateTagSpecifications(),
	})
	if err != nil {
		return fmt.Errorf("error while creating subnet: %w", err)
	}
	f.subnetId = *createSubnetOutput.Subnet.SubnetId

	createRouteTableOutput, err := f.ec2Client.CreateRouteTable(&ec2.CreateRouteTableInput{
		VpcId: awssdk.String(f.vpcId),
	})
	if err != nil {
		return fmt.Errorf("error while creating route table: %w", err)
	}
	f.routeTableId = *createRouteTableOutput.RouteTable.RouteTableId

	assocRouteTableOutput, err := f.ec2Client.AssociateRouteTable(&ec2.AssociateRouteTableInput{
		RouteTableId: awssdk.String(f.routeTableId),
		SubnetId:     awssdk.String(f.subnetId),
	})
	if err != nil {
		return fmt.Errorf("error while associating route table with subnet: %w", err)
	}
	f.associtationID = *assocRouteTableOutput.AssociationId

	f.managementClusterRoleIdentity = &capa.AWSClusterRoleIdentity{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: capa.AWSClusterRoleIdentitySpec{
			AWSRoleSpec: capa.AWSRoleSpec{
				RoleArn: getRoleARN(f.managementAWSAccount, f.awsIAMRoleId),
			},
			SourceIdentityRef: &capa.AWSIdentityReference{
				Name: "default",
				Kind: capa.ControllerIdentityKind,
			},
		},
	}

	err = f.k8sClient.Create(ctx, f.managementClusterRoleIdentity)
	if err != nil {
		return fmt.Errorf("error while creating AWSClusterRoleIdentity: %w", err)
	}

	f.managementCluster = &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mc",
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
				Name:       "test-mc",
			},
		},
	}
	f.managementAWSCluster = &capa.AWSCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mc",
			Namespace: "test",
		},
		Spec: capa.AWSClusterSpec{
			IdentityRef: &capa.AWSIdentityReference{
				Name: name,
				Kind: capa.ClusterRoleIdentityKind,
			},
			NetworkSpec: capa.NetworkSpec{
				VPC: capa.VPCSpec{
					ID:        f.vpcId,
					CidrBlock: "172.64.0.0/16",
				},
				Subnets: []capa.SubnetSpec{
					{
						CidrBlock: "172.64.0.0/20",
						ID:        f.subnetId,
						IsPublic:  false,
					},
				},
			},
		},
	}

	err = f.k8sClient.Create(ctx, f.managementCluster)
	if err != nil {
		return fmt.Errorf("error while creating Cluster: %w", err)
	}

	err = f.k8sClient.Create(ctx, f.managementAWSCluster)
	if err != nil {
		return fmt.Errorf("error while creating AWSCluster: %w", err)
	}

	return nil
}

func (f *Fixture) CreateWCOnAnotherAccount(ctx context.Context) error {
	createVpcOutput, err := f.ec2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: awssdk.String("172.32.0.0/16"),
	})
	if err != nil {
		return fmt.Errorf("error while creating vpc: %w", err)
	}

	f.vpcIdWC = *createVpcOutput.Vpc.VpcId

	createSubnetOutput, err := f.ec2Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock:         awssdk.String("172.32.0.0/20"),
		VpcId:             awssdk.String(f.vpcIdWC),
		AvailabilityZone:  awssdk.String(getAvailabilityZone(f.awsRegion)),
		TagSpecifications: generateTagSpecifications(),
	})
	if err != nil {
		return fmt.Errorf("error while creating subnet: %w", err)
	}
	f.subnetIdWC = *createSubnetOutput.Subnet.SubnetId

	name := tests.GenerateGUID("test-share")
	f.workloadClusterRoleIdentity = &capa.AWSClusterRoleIdentity{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: capa.AWSClusterRoleIdentitySpec{
			AWSRoleSpec: capa.AWSRoleSpec{
				RoleArn: getRoleARN(f.workloadAWSAccount, f.awsIAMRoleId),
			},
			SourceIdentityRef: &capa.AWSIdentityReference{
				Name: "default",
				Kind: capa.ControllerIdentityKind,
			},
		},
	}

	err = f.k8sClient.Create(ctx, f.workloadClusterRoleIdentity)
	if err != nil {
		return fmt.Errorf("error while creating AWSClusterRoleIdentity: %w", err)
	}

	f.workloadCluster = &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-wc-share",
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
				Name:       "test-wc-share",
			},
		},
	}
	f.workloadAWSCluster = &capa.AWSCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-wc-share",
			Namespace: "test",
		},
		Spec: capa.AWSClusterSpec{
			IdentityRef: &capa.AWSIdentityReference{
				Name: name,
				Kind: capa.ClusterRoleIdentityKind,
			},
			NetworkSpec: capa.NetworkSpec{
				VPC: capa.VPCSpec{
					ID:        f.vpcIdWC,
					CidrBlock: "172.32.0.0/16",
				},
				Subnets: []capa.SubnetSpec{
					{
						CidrBlock: "172.32.0.0/20",
						ID:        f.subnetIdWC,
						IsPublic:  false,
					},
				},
			},
		},
	}

	err = f.k8sClient.Create(ctx, f.workloadCluster)
	if err != nil {
		return fmt.Errorf("error while creating Cluster: %w", err)
	}

	err = f.k8sClient.Create(ctx, f.workloadAWSCluster)
	if err != nil {
		return fmt.Errorf("error while creating AWSCluster: %w", err)
	}

	return nil
}

func (f *Fixture) Teardown(ctx context.Context, k8sClient client.Client, rawEC2Client *ec2.EC2) error {
	_, err := rawEC2Client.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
		AssociationId: &f.associtationID,
	})
	if err != nil {
		return fmt.Errorf("error while disassociating route table with subnet: %w", err)
	}

	_, err = rawEC2Client.DeleteRouteTable(&ec2.DeleteRouteTableInput{
		RouteTableId: &f.routeTableId,
	})
	if err != nil {
		return fmt.Errorf("error while deleting route table : %w", err)
	}

	_, err = rawEC2Client.DeleteSubnet(&ec2.DeleteSubnetInput{
		SubnetId: awssdk.String(f.subnetId),
	})
	if err != nil {
		return fmt.Errorf("error while MC deleting subnet: %w", err)
	}

	_, err = rawEC2Client.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: awssdk.String(f.vpcId),
	})
	if err != nil {
		return fmt.Errorf("error while MC deleting vpc: %w", err)
	}

	_, err = rawEC2Client.DeleteSubnet(&ec2.DeleteSubnetInput{
		SubnetId: awssdk.String(f.subnetIdWC),
	})
	if err != nil {
		return fmt.Errorf("error while WC deleting subnet: %w", err)
	}

	_, err = rawEC2Client.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: awssdk.String(f.vpcIdWC),
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

func generateTagSpecifications() []*ec2.TagSpecification {
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
				Key:   awssdk.String(capa.NameKubernetesAWSCloudProviderPrefix + "test-mc"),
				Value: awssdk.String("shared"),
			},
		},
	}

	tagSpecifications := make([]*ec2.TagSpecification, 0)
	tagSpecifications = append(tagSpecifications, tagSpec)
	return tagSpecifications
}

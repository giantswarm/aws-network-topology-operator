package acceptance

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	gsannotations "github.com/giantswarm/k8smetadata/pkg/annotation"
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

type Fixture struct {
	managementCluster    *capi.Cluster
	managementAWSCluster *capa.AWSCluster
	clusterRoleIdentity  *capa.AWSClusterRoleIdentity
	subnetId             string
	vpcId                string
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

func (f *Fixture) GetClusterRoleIdentity() *capa.AWSClusterRoleIdentity {
	return f.clusterRoleIdentity
}
func (f *Fixture) GetVpcID() string {
	return f.vpcId
}

func (f *Fixture) Setup(ctx context.Context, k8sClient client.Client, rawEC2Client *ec2.EC2, mcIAMRoleARN, awsRegion, availabilityZone string) error {
	name := tests.GenerateGUID("test")

	createVpcOutput, err := rawEC2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: aws.String("172.64.0.0/16"),
	})
	if err != nil {
		return fmt.Errorf("error while creating vpc: %w", err)
	}

	f.vpcId = *createVpcOutput.Vpc.VpcId

	createSubnetOutput, err := rawEC2Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock:         aws.String("172.64.0.0/20"),
		VpcId:             aws.String(f.vpcId),
		AvailabilityZone:  &availabilityZone,
		TagSpecifications: generateTagSpecifications(),
	})
	if err != nil {
		return fmt.Errorf("error while creating subnet: %w", err)
	}
	f.subnetId = *createSubnetOutput.Subnet.SubnetId

	f.clusterRoleIdentity = &capa.AWSClusterRoleIdentity{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: capa.AWSClusterRoleIdentitySpec{
			AWSRoleSpec: capa.AWSRoleSpec{
				RoleArn: mcIAMRoleARN,
			},
			SourceIdentityRef: &capa.AWSIdentityReference{
				Name: "default",
				Kind: capa.ControllerIdentityKind,
			},
		},
	}

	err = k8sClient.Create(ctx, f.clusterRoleIdentity)
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

	err = k8sClient.Create(ctx, f.managementCluster)
	if err != nil {
		return fmt.Errorf("error while creating Cluster: %w", err)
	}

	err = k8sClient.Create(ctx, f.managementAWSCluster)
	if err != nil {
		return fmt.Errorf("error while creating AWSCluster: %w", err)
	}

	return nil
}

func (f *Fixture) Teardown(ctx context.Context, k8sClient client.Client, rawEC2Client *ec2.EC2) error {
	_, err := rawEC2Client.DeleteSubnet(&ec2.DeleteSubnetInput{
		SubnetId: aws.String(f.subnetId),
	})
	if err != nil {
		return fmt.Errorf("error while deleting subnets: %w", err)
	}

	_, err = rawEC2Client.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: aws.String(f.vpcId),
	})
	if err != nil {
		return fmt.Errorf("error while deleting vpcs: %w", err)
	}

	return nil
}

func generateTagSpecifications() []*ec2.TagSpecification {
	tagSpec := &ec2.TagSpecification{
		ResourceType: aws.String(ec2.ResourceTypeSubnet),
		Tags: []*ec2.Tag{
			{
				Key:   aws.String(registrar.SubnetTGWAttachementsLabel),
				Value: aws.String("true"),
			},
			{
				Key:   aws.String(registrar.SubnetRoleLabel),
				Value: aws.String("private"),
			},
			{
				Key:   aws.String(capa.NameKubernetesAWSCloudProviderPrefix + "test-mc"),
				Value: aws.String("shared"),
			},
		},
	}

	tagSpecifications := make([]*ec2.TagSpecification, 0)
	tagSpecifications = append(tagSpecifications, tagSpec)
	return tagSpecifications
}

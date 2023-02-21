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

func (f *Fixture) GetManagementAWSCluster() *capa.AWSCluster {
	return f.managementAWSCluster
}

func (f *Fixture) GetClusterRoleIdentity() *capa.AWSClusterRoleIdentity {
	return f.clusterRoleIdentity
}

func (f *Fixture) Setup(ctx context.Context, k8sClient client.Client, rawEC2Client *ec2.EC2, mcIAMRoleARN, awsRegion, availabilityZone string) error {
	name := tests.GenerateGUID("test")

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

	err := k8sClient.Create(ctx, f.clusterRoleIdentity)
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

	createVpcOutput, err := rawEC2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: aws.String("172.64.0.0/16"),
	})
	if err != nil {
		return fmt.Errorf("error while creating vpc: %w", err)
	}

	f.vpcId = *createVpcOutput.Vpc.VpcId
	createSubnetOutput, err := rawEC2Client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock:        aws.String("172.64.0.0/20"),
		VpcId:            createVpcOutput.Vpc.VpcId,
		AvailabilityZone: &availabilityZone,
	})
	if err != nil {
		return fmt.Errorf("error while creating subnet: %w", err)
	}
	f.subnetId = *createSubnetOutput.Subnet.SubnetId

	return nil
}

func (f *Fixture) Teardown(ctx context.Context, k8sClient client.Client, rawEC2Client *ec2.EC2) error {
	/*	err := k8sClient.Delete(ctx, f.managementCluster)
		if err != nil {
			return err
		}

		err = k8sClient.Delete(ctx, f.managementAWSCluster)
		if !k8serrors.IsNotFound(err) {
			if err != nil {
				return err
			}
		}

		err = k8sClient.Delete(ctx, f.clusterRoleIdentity)
		if !k8serrors.IsNotFound(err) {
			if err != nil {
				return err
			}
		}

		controllerutil.RemoveFinalizer(f.managementCluster, capa.ClusterFinalizer)
		patchHelper, err := patch.NewHelper(f.managementCluster, k8sClient)
		if err != nil {
			return err
		}

		err = patchHelper.Patch(ctx, f.managementCluster)
		if err != nil {
			return err
		}*/

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

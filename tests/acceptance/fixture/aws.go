package fixture

import (
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
)

const (
	ManagementClusterCIDR = "172.64.0.0"
	WorkloadClusterCIDR   = "172.96.0.0"
)

func DisassociateRouteTable(ec2Client *ec2.EC2, associationID string) error {
	if associationID == "" {
		return nil
	}

	_, err := ec2Client.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
		AssociationId: awssdk.String(associationID),
	})

	if aws.HasErrorCode(err, aws.ErrAssociationNotFound) {
		return nil
	}

	return err
}

func DeleteRouteTable(ec2Client *ec2.EC2, routeTableID string) error {
	if routeTableID == "" {
		return nil
	}

	_, err := ec2Client.DeleteRouteTable(&ec2.DeleteRouteTableInput{
		RouteTableId: awssdk.String(routeTableID),
	})
	if aws.HasErrorCode(err, aws.ErrRouteTableNotFound) {
		return nil
	}

	return err
}

func DeleteSubnet(ec2Client *ec2.EC2, subnetID string) error {
	if subnetID == "" {
		return nil
	}

	_, err := ec2Client.DeleteSubnet(&ec2.DeleteSubnetInput{
		SubnetId: awssdk.String(subnetID),
	})
	if aws.HasErrorCode(err, aws.ErrSubnetNotFound) {
		return nil
	}

	return err
}

func CreateVPC(ec2Client *ec2.EC2, cidr string) (string, error) {
	vpcCIDR := fmt.Sprintf("%s/%d", cidr, 16)

	output, err := ec2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: awssdk.String(vpcCIDR),
	})
	if err != nil {
		return "", fmt.Errorf("error while creating vpc: %w", err)
	}

	return *output.Vpc.VpcId, nil
}

func DeleteVPC(ec2Client *ec2.EC2, vpcID string) error {
	if vpcID == "" {
		return nil
	}

	_, err := ec2Client.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: awssdk.String(vpcID),
	})
	if aws.HasErrorCode(err, aws.ErrVPCNotFound) {
		return nil
	}

	return err
}

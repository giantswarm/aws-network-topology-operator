package registrar

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	k8stypes "k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
)

type contextKey string

var clusterNameContextKey contextKey = "clusterName"

//counterfeiter:generate . TransitGatewayClient
type TransitGatewayClient interface {
	CreateTransitGateway(ctx context.Context, params *ec2.CreateTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayOutput, error)
	CreateTransitGatewayVpcAttachment(ctx context.Context, params *ec2.CreateTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error)
	DeleteTransitGateway(ctx context.Context, params *ec2.DeleteTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayOutput, error)
	DeleteTransitGatewayVpcAttachment(ctx context.Context, params *ec2.DeleteTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error)
	DescribeTransitGateways(ctx context.Context, params *ec2.DescribeTransitGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewaysOutput, error)
	DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error)
}

//counterfeiter:generate . ClusterClient
type ClusterClient interface {
	Save(ctx context.Context, cluster *capi.Cluster) (*capi.Cluster, error)
	GetManagementCluster(ctx context.Context) (*capi.Cluster, error)
	GetAWSCluster(ctx context.Context, namespacedName k8stypes.NamespacedName) (*capa.AWSCluster, error)
	IsManagementCluster(ctx context.Context, cluster *capi.Cluster) bool
}

type TransitGateway struct {
	transitGatewayClient TransitGatewayClient
	clusterClient        ClusterClient
}

func NewTransitGateway(transitGatewayClient TransitGatewayClient) *TransitGateway {
	return &TransitGateway{
		transitGatewayClient: transitGatewayClient,
	}
}

func (r *TransitGateway) Register(ctx context.Context, cluster *capi.Cluster) error {
	ctx = context.WithValue(ctx, clusterNameContextKey, cluster.ObjectMeta.Name)
	logger := r.getLogger(ctx)

	gatewayID := annotations.GetNetworkTopologyTransitGatewayID(cluster)

	switch val := annotations.GetAnnotation(cluster, annotations.NetworkTopologyModeAnnotation); val {
	case "":
		// If no value currently set, we'll set to the default of 'None"
		logger.Info("NetworkTopologyMode is currently unset, setting to the default of 'None'")
		annotations.AddAnnotations(cluster, map[string]string{
			annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeNone,
		})
		if _, err := r.clusterClient.Save(ctx, cluster); err != nil {
			logger.Error(err, "Failed to save cluster resource")
			return err
		}
		fallthrough

	case annotations.NetworkTopologyModeNone:
		// TODO: Handle `None` topology mode
		logger.Info("Mode currently not handled", "mode", annotations.NetworkTopologyModeNone)

	case annotations.NetworkTopologyModeUserManaged:
		// TODO: Handle `UserManaged` mode
		logger.Info("Mode currently not handled", "mode", annotations.NetworkTopologyModeUserManaged)

	case annotations.NetworkTopologyModeGiantSwarmManaged:
		var err error
		var tgw *types.TransitGateway

		if r.clusterClient.IsManagementCluster(ctx, cluster) {
			tgw, err = r.getOrCreateTransitGateway(ctx, gatewayID)
			if err != nil {
				return err
			}
		} else {
			if gatewayID == "" {
				// No TGW ID specified so we'll use the one associated with the MC
				mc, err := r.clusterClient.GetManagementCluster(ctx)
				if err != nil {
					logger.Error(err, "Failed to get management cluster")
					return err
				}

				gatewayID = annotations.GetNetworkTopologyTransitGatewayID(mc)
				if gatewayID == "" {
					err = fmt.Errorf("management cluster doesn't have a TGW specified")
					logger.Error(err, "The Management Cluster doesn't have a Transit Gateway ID specified")
					return err
				}
			}

			tgw, err = r.getTransitGateway(ctx, gatewayID)
			if err != nil {
				return err
			}
			if tgw == nil {
				err = fmt.Errorf("failed to find TransitGateway for provided ID")
				logger.Error(err, "No TransitGateway found for ID provided on annotations", "transitGatewayID", gatewayID)
				return err
			}
		}

		// Ensure TGW ID is saved back to the current cluster
		annotations.SetNetworkTopologyTransitGatewayID(cluster, *tgw.TransitGatewayId)
		if cluster, err = r.clusterClient.Save(ctx, cluster); err != nil {
			logger.Error(err, "Failed to save cluster resource")
			return err
		}

		if err := r.attachTransitGateway(ctx, tgw.TransitGatewayId, cluster); err != nil {
			logger.Error(err, "Failed to attach transit gateway to cluster VPC")
			return err
		}

	default:
		err := fmt.Errorf("invalid NetworkTopologyMode value")
		logger.Error(err, "Unexpected NetworkTopologyMode annotation value found on cluster", "value", val)
		return err
	}

	logger.Info("Done Registering TransitGateway")
	return nil
}

func (r *TransitGateway) Unregister(ctx context.Context, cluster *capi.Cluster) error {
	logger := r.getLogger(ctx)

	gatewayID := annotations.GetNetworkTopologyTransitGatewayID(cluster)

	switch val := annotations.GetAnnotation(cluster, annotations.NetworkTopologyModeAnnotation); val {
	case annotations.NetworkTopologyModeNone:
		// TODO: Handle `None` topology mode
		logger.Info("Mode currently not handled", "mode", annotations.NetworkTopologyModeNone)

	case annotations.NetworkTopologyModeUserManaged:
		// TODO: Handle `UserManaged` mode
		logger.Info("Mode currently not handled", "mode", annotations.NetworkTopologyModeUserManaged)

	case annotations.NetworkTopologyModeGiantSwarmManaged:
		if err := r.detachTransitGateway(ctx, &gatewayID, cluster); err != nil {
			return err
		}

	default:
		err := fmt.Errorf("invalid NetworkTopologyMode value")
		logger.Error(err, "Unexpected NetworkTopologyMode annotation value found on cluster", "value", val)
		return err
	}

	logger.Info("Done unregistering TransitGateway")
	return nil
}

func (r *TransitGateway) getLogger(ctx context.Context) logr.Logger {
	logger := log.FromContext(ctx)
	return logger.WithName("transitgateway-registrar")
}

func (r *TransitGateway) getTransitGateway(ctx context.Context, gatewayID string) (*types.TransitGateway, error) {
	logger := r.getLogger(ctx)

	var tgw *types.TransitGateway

	if gatewayID != "" {
		describeOutput, err := r.transitGatewayClient.DescribeTransitGateways(ctx, &ec2.DescribeTransitGatewaysInput{
			TransitGatewayIds: []string{gatewayID},
		})
		if err != nil {
			logger.Error(err, "Failed to describe TransitGateways", "transitGatewayID", gatewayID)
			return tgw, err
		}

		if len(describeOutput.TransitGateways) > 1 {
			err = fmt.Errorf("multiple Transit Gateways found for ID, expected at most one")
			logger.Error(err, "Too many Transit Gateways found for ID", "transitGatewayID", gatewayID)
			return tgw, err
		} else if len(describeOutput.TransitGateways) == 1 {
			tgw = &describeOutput.TransitGateways[0]
		}

		logger.Info("Got TransitGateway", "transitGatewayID", tgw.TransitGatewayId)
	}

	return tgw, nil
}

func (r *TransitGateway) getOrCreateTransitGateway(ctx context.Context, gatewayID string) (*types.TransitGateway, error) {
	logger := r.getLogger(ctx)

	tgw, err := r.getTransitGateway(ctx, gatewayID)
	if err != nil {
		return nil, err
	}

	if tgw == nil {
		logger.Info("No existing TGW found, creating a new one")

		output, err := r.transitGatewayClient.CreateTransitGateway(ctx, &ec2.CreateTransitGatewayInput{
			Description: aws.String(fmt.Sprintf("Transit Gateway for cluster %s", ctx.Value(clusterNameContextKey))),
			Options: &types.TransitGatewayRequestOptions{
				AutoAcceptSharedAttachments: types.AutoAcceptSharedAttachmentsValueEnable,
			},
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeTransitGateway,
					Tags: []types.Tag{
						{
							Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", ctx.Value(clusterNameContextKey))),
							Value: aws.String("owned"),
						},
					},
				},
			},
		})
		if err != nil {
			logger.Error(err, "Failed to create new Transit Gateway")
			return nil, err
		}

		logger.Info("Created new TransitGateway", "transitGatewayID", tgw.TransitGatewayId)
		return output.TransitGateway, nil
	}

	return tgw, nil
}

func (r *TransitGateway) attachTransitGateway(ctx context.Context, gatewayID *string, cluster *capi.Cluster) error {
	logger := r.getLogger(ctx)

	clusterNamespaceName := k8stypes.NamespacedName{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Spec.InfrastructureRef.Namespace,
	}
	awsCluster, err := r.clusterClient.GetAWSCluster(ctx, clusterNamespaceName)
	if err != nil {
		logger.Error(err, "Failed to get AWSCluster for Cluster")
		return err
	}

	vpcID := awsCluster.Spec.NetworkSpec.VPC.ID
	subnets := []string{}
	for _, s := range awsCluster.Spec.NetworkSpec.Subnets {
		subnets = append(subnets, s.ID)
	}
	output, err := r.transitGatewayClient.CreateTransitGatewayVpcAttachment(ctx, &ec2.CreateTransitGatewayVpcAttachmentInput{
		TransitGatewayId: gatewayID,
		VpcId:            &vpcID,
		SubnetIds:        subnets,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeTransitGateway,
				Tags: []types.Tag{
					{
						Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", ctx.Value(clusterNameContextKey))),
						Value: aws.String("owned"),
					},
				},
			},
		},
	})

	if err != nil {
		return err
	}

	logger.Info("TransitGateway attached to VPC", "vpcID", vpcID, "transitGatewayID", gatewayID, "transitGatewayAttachmentId", output.TransitGatewayVpcAttachment.TransitGatewayAttachmentId)
	return nil
}

func (r *TransitGateway) detachTransitGateway(ctx context.Context, gatewayID *string, cluster *capi.Cluster) error {
	logger := r.getLogger(ctx)

	clusterNamespaceName := k8stypes.NamespacedName{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Spec.InfrastructureRef.Namespace,
	}
	awsCluster, err := r.clusterClient.GetAWSCluster(ctx, clusterNamespaceName)
	if err != nil {
		logger.Error(err, "Failed to get AWSCluster for Cluster")
		return err
	}

	vpcID := awsCluster.Spec.NetworkSpec.VPC.ID

	describeTGWattachmentInput := &ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("transit-gateway-id"),
				Values: []string{*gatewayID},
			},
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	}

	attachments, err := r.transitGatewayClient.DescribeTransitGatewayVpcAttachments(ctx, describeTGWattachmentInput)
	if err != nil {
		return err
	}

	for _, tgwAttachment := range attachments.TransitGatewayVpcAttachments {
		_, err := r.transitGatewayClient.DeleteTransitGatewayVpcAttachment(ctx, &ec2.DeleteTransitGatewayVpcAttachmentInput{
			TransitGatewayAttachmentId: tgwAttachment.TransitGatewayAttachmentId,
		})
		if err != nil {
			logger.Error(err, "Failed to delete TransitGatewayAttachment", "transitGatewayID", gatewayID, "vpcID", vpcID, "transitGatewayAttachmentID", tgwAttachment.TransitGatewayAttachmentId)
			return err
		}
	}

	logger.Info("TransitGateway detached from VPC", "vpcID", vpcID, "transitGatewayID", gatewayID)
	return nil
}

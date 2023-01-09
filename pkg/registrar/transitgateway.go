package registrar

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/giantswarm/k8smetadata/pkg/annotation"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsclient "github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
)

type contextKey string

var clusterNameContextKey contextKey = "clusterName"

const (
	// PREFIX_LIST_MAX_ENTRIES is the maximum number of entries a created prefix list can have.
	// This number counts against a resources quota (regardless of how many actual entries exist)
	// when it is referenced. We're setting the max here to 45 for now so we stay below the
	// default "Routes per route table" quota of 50.
	PREFIX_LIST_MAX_ENTRIES = 45
)

//counterfeiter:generate . ClusterClient
type ClusterClient interface {
	Patch(ctx context.Context, cluster *capi.Cluster, patch client.Patch) (*capi.Cluster, error)
	GetManagementCluster(ctx context.Context) (*capi.Cluster, error)
	GetManagementClusterNamespacedName() k8stypes.NamespacedName
	GetAWSCluster(ctx context.Context, namespacedName k8stypes.NamespacedName) (*capa.AWSCluster, error)
	IsManagementCluster(ctx context.Context, cluster *capi.Cluster) bool
}

type TransitGateway struct {
	transitGatewayClient                      awsclient.TransitGatewayClient
	clusterClient                             ClusterClient
	getTransitGatewayClientForWorkloadCluster func(workloadCluster k8stypes.NamespacedName) awsclient.TransitGatewayClient
}

func NewTransitGateway(transitGatewayClient awsclient.TransitGatewayClient, clusterClient ClusterClient, getTransitGatewayClientForWorkloadCluster func(workloadCluster k8stypes.NamespacedName) awsclient.TransitGatewayClient) *TransitGateway {
	return &TransitGateway{
		transitGatewayClient: transitGatewayClient,
		clusterClient:        clusterClient,
		getTransitGatewayClientForWorkloadCluster: getTransitGatewayClientForWorkloadCluster,
	}
}

func (r *TransitGateway) Register(ctx context.Context, cluster *capi.Cluster) error {
	ctx = context.WithValue(ctx, clusterNameContextKey, cluster.ObjectMeta.Name)
	logger := r.getLogger(ctx)

	gatewayID := annotations.GetNetworkTopologyTransitGatewayID(cluster)

	switch val := annotations.GetAnnotation(cluster, annotation.NetworkTopologyModeAnnotation); val {
	case "":
		// If no value currently set, we'll set to the default of 'None"
		logger.Info("NetworkTopologyMode is currently unset, setting to the default of 'None'")

		baseCluster := cluster.DeepCopy()
		annotations.AddAnnotations(cluster, map[string]string{
			annotation.NetworkTopologyModeAnnotation: annotation.NetworkTopologyModeNone,
		})
		if _, err := r.clusterClient.Patch(ctx, cluster, client.MergeFrom(baseCluster)); err != nil {
			logger.Error(err, "Failed to save cluster resource")
			return err
		}
		fallthrough

	case annotation.NetworkTopologyModeNone:
		logger.Info("Mode currently not handled", "mode", annotation.NetworkTopologyModeNone)
		return &ModeNotSupportedError{Mode: val}

	case annotation.NetworkTopologyModeUserManaged:
		var err error
		var tgw *types.TransitGateway

		prefixListID := annotations.GetNetworkTopologyPrefixListID(cluster)
		if prefixListID == "" {
			return &IDNotProvidedError{ID: "PrefixList"}
		}

		if r.clusterClient.IsManagementCluster(ctx, cluster) {
			if gatewayID == "" {
				return &IDNotProvidedError{ID: "TransitGateway"}
			}

			tgw, err = r.getTransitGateway(ctx, gatewayID)
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
		baseCluster := cluster.DeepCopy()
		annotations.SetNetworkTopologyTransitGatewayID(cluster, *tgw.TransitGatewayId)
		if cluster, err = r.clusterClient.Patch(ctx, cluster, client.MergeFrom(baseCluster)); err != nil {
			logger.Error(err, "Failed to patch cluster resource with TGW ID")
			return err
		}

		awsCluster, err := r.getAWSCluster(ctx, cluster)
		if err != nil {
			logger.Error(err, "Failed to get AWSCluster for Cluster")
			return err
		}

		var tgwAttachment *types.TransitGatewayVpcAttachment
		if awsCluster.Spec.NetworkSpec.VPC.ID == "" {
			logger.Info("vpc not yet ready, skipping attachment for now", "transitGatewayID", tgw.TransitGatewayId)
			return &VPCNotReadyError{}
		} else if tgw.State == types.TransitGatewayStateAvailable {
			tgwAttachment, err = r.attachTransitGateway(ctx, tgw.TransitGatewayId, awsCluster)
			if err != nil {
				return err
			}
		} else {
			logger.Info("transit gateway not available, skipping attachment for now", "transitGatewayID", tgw.TransitGatewayId, "tgwState", tgw.State)
			return &TransitGatewayNotAvailableError{}
		}

		if tgwAttachment.State == types.TransitGatewayAttachmentStateInitiating || tgwAttachment.State == types.TransitGatewayAttachmentStateInitiatingRequest ||
			tgwAttachment.State == types.TransitGatewayAttachmentStatePending || tgwAttachment.State == types.TransitGatewayAttachmentStatePendingAcceptance {
			logger.Info("Sending SNS message")

			_, err = r.transitGatewayClient.PublishSNSMessage(ctx, &sns.PublishInput{
				Message: aws.String("Request TransitGatewayAttachment"),
				MessageAttributes: map[string]snstypes.MessageAttributeValue{
					"Postfach":      {DataType: aws.String("String"), StringValue: aws.String("support@giantswarm.io")},
					"Account_ID":    {DataType: aws.String("String"), StringValue: tgwAttachment.VpcOwnerId},
					"Attachment_ID": {DataType: aws.String("String"), StringValue: tgwAttachment.TransitGatewayAttachmentId},
					"CIDR":          {DataType: aws.String("String"), StringValue: &awsCluster.Spec.NetworkSpec.VPC.CidrBlock},
					"Name":          {DataType: aws.String("String"), StringValue: &cluster.ObjectMeta.Name},
				},
			})
			if err != nil {
				logger.Error(err, "Failed sending SNS message")
				return err
			}
		}

		if err := r.addRoutes(ctx, tgw.TransitGatewayId, &prefixListID, awsCluster); err != nil {
			return err
		}

	case annotation.NetworkTopologyModeGiantSwarmManaged:
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
		baseCluster := cluster.DeepCopy()
		annotations.SetNetworkTopologyTransitGatewayID(cluster, *tgw.TransitGatewayId)
		if cluster, err = r.clusterClient.Patch(ctx, cluster, client.MergeFrom(baseCluster)); err != nil {
			logger.Error(err, "Failed to patch cluster resource with TGW ID")
			return err
		}

		awsCluster, err := r.getAWSCluster(ctx, cluster)
		if err != nil {
			logger.Error(err, "Failed to get AWSCluster for Cluster")
			return err
		}

		if awsCluster.Spec.NetworkSpec.VPC.ID == "" {
			logger.Info("vpc not yet ready, skipping attachment for now", "transitGatewayID", tgw.TransitGatewayId)
			return &VPCNotReadyError{}
		} else if tgw.State == types.TransitGatewayStateAvailable {
			if _, err := r.attachTransitGateway(ctx, tgw.TransitGatewayId, awsCluster); err != nil {
				return err
			}
		} else {
			logger.Info("transit gateway not available, skipping attachment for now", "transitGatewayID", tgw.TransitGatewayId, "tgwState", tgw.State)
			return &TransitGatewayNotAvailableError{}
		}

		prefixListID, err := r.addToPrefixList(ctx, awsCluster)
		if err != nil {
			return err
		}

		// Ensure PrefixListID is saved back to the current cluster
		baseCluster = cluster.DeepCopy()
		annotations.SetNetworkTopologyPrefixListID(cluster, prefixListID)
		if _, err = r.clusterClient.Patch(ctx, cluster, client.MergeFrom(baseCluster)); err != nil {
			logger.Error(err, "Failed to patch cluster resource with prefix list ID", "prefixListID", prefixListID)
			return err
		}

		if err := r.addRoutes(ctx, tgw.TransitGatewayId, &prefixListID, awsCluster); err != nil {
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

	switch val := annotations.GetAnnotation(cluster, annotation.NetworkTopologyModeAnnotation); val {
	case annotation.NetworkTopologyModeNone:
		logger.Info("Mode currently not handled", "mode", annotation.NetworkTopologyModeNone)

	case annotation.NetworkTopologyModeUserManaged:
		awsCluster, err := r.getAWSCluster(ctx, cluster)
		if errors.IsNotFound(err) {
			logger.Info("AWSCluster is already deleted, skipping transit gateway deletion")
			return nil
		} else if err != nil {
			logger.Error(err, "Failed to get AWSCluster for Cluster")
			return err
		}

		if err := r.detachTransitGateway(ctx, &gatewayID, awsCluster); err != nil {
			return err
		}

	case annotation.NetworkTopologyModeGiantSwarmManaged:
		awsCluster, err := r.getAWSCluster(ctx, cluster)
		if errors.IsNotFound(err) {
			logger.Info("AWSCluster is already deleted, skipping transit gateway deletion")
			return nil
		} else if err != nil {
			logger.Error(err, "Failed to get AWSCluster for Cluster")
			return err
		}

		if err := r.removeFromPrefixList(ctx, awsCluster); err != nil {
			return err
		}

		if err := r.removeRoutes(ctx, awsCluster); err != nil {
			return err
		}

		if err := r.detachTransitGateway(ctx, &gatewayID, awsCluster); err != nil {
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

func (r *TransitGateway) getAWSCluster(ctx context.Context, cluster *capi.Cluster) (*capa.AWSCluster, error) {
	clusterNamespaceName := k8stypes.NamespacedName{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Spec.InfrastructureRef.Namespace,
	}
	return r.clusterClient.GetAWSCluster(ctx, clusterNamespaceName)
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
			logger.Info("Got TransitGateway", "transitGatewayID", tgw.TransitGatewayId)
		}
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

		tgw = output.TransitGateway

		logger.Info("Created new TransitGateway", "transitGatewayID", tgw.TransitGatewayId)
	}

	return tgw, nil
}

func (r *TransitGateway) attachTransitGateway(ctx context.Context, gatewayID *string, awsCluster *capa.AWSCluster) (*types.TransitGatewayVpcAttachment, error) {
	logger := r.getLogger(ctx)

	vpcID := awsCluster.Spec.NetworkSpec.VPC.ID
	subnets := []string{}
	for _, s := range getPrivateSubnetsByAZ(awsCluster.Spec.NetworkSpec.Subnets) {
		subnets = append(subnets, s[0].ID)
	}

	if vpcID == "" || len(subnets) == 0 {
		err := fmt.Errorf("cluster network not yet available on AWSCluster resource")
		logger.Error(err, "AWSCluster does not yet have network details available")
		return nil, err
	}

	// Attachments from VPC to the transit gateway need to be made from the AWS account
	// of the workload cluster, so we use a separate client
	transitGatewayAttachmentClient := r.getTransitGatewayClientForWorkloadCluster(k8stypes.NamespacedName{
		Name:      awsCluster.ObjectMeta.Name,
		Namespace: awsCluster.ObjectMeta.Namespace,
	})

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
	attachments, err := transitGatewayAttachmentClient.DescribeTransitGatewayVpcAttachments(ctx, describeTGWattachmentInput)
	if err != nil {
		logger.Error(err, "Failed to get transit gateway attachments", "transitGatewayID", gatewayID)
		return nil, err
	}

	if attachments != nil && len(attachments.TransitGatewayVpcAttachments) == 0 {
		output, err := transitGatewayAttachmentClient.CreateTransitGatewayVpcAttachment(ctx, &ec2.CreateTransitGatewayVpcAttachmentInput{
			TransitGatewayId: gatewayID,
			VpcId:            &vpcID,
			SubnetIds:        subnets,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeTransitGatewayAttachment,
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
			logger.Error(err, "Failed to create transit gateway attachments", "transitGatewayID", gatewayID, "vpcID", vpcID)
			return nil, err
		}

		logger.Info("TransitGateway attached to VPC", "vpcID", vpcID, "transitGatewayID", gatewayID, "transitGatewayAttachmentId", output.TransitGatewayVpcAttachment.TransitGatewayAttachmentId)
		return output.TransitGatewayVpcAttachment, nil
	} else if len(attachments.TransitGatewayVpcAttachments) == 1 {
		return &attachments.TransitGatewayVpcAttachments[0], nil
	}

	return nil, nil
}

func (r *TransitGateway) detachTransitGateway(ctx context.Context, gatewayID *string, awsCluster *capa.AWSCluster) error {
	logger := r.getLogger(ctx)

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

	transitGatewayAttachmentClient := r.getTransitGatewayClientForWorkloadCluster(k8stypes.NamespacedName{
		Name:      awsCluster.ObjectMeta.Name,
		Namespace: awsCluster.ObjectMeta.Namespace,
	})

	attachments, err := transitGatewayAttachmentClient.DescribeTransitGatewayVpcAttachments(ctx, describeTGWattachmentInput)
	if err != nil {
		logger.Error(err, "Failed to get transit gateway attachments", "transitGatewayID", gatewayID)
		return err
	}

	for _, tgwAttachment := range attachments.TransitGatewayVpcAttachments {
		_, err := transitGatewayAttachmentClient.DeleteTransitGatewayVpcAttachment(ctx, &ec2.DeleteTransitGatewayVpcAttachmentInput{
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

func (r *TransitGateway) getOrCreatePrefixList(ctx context.Context) (*types.ManagedPrefixList, error) {
	logger := r.getLogger(ctx)

	mc, err := r.clusterClient.GetManagementCluster(ctx)
	if err != nil {
		logger.Error(err, "Failed to get management cluster")
		return nil, err
	}

	prefixListID := annotations.GetNetworkTopologyPrefixListID(mc)

	if prefixListID != "" {
		result, err := r.transitGatewayClient.DescribeManagedPrefixLists(ctx, &ec2.DescribeManagedPrefixListsInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("prefix-list-id"),
					Values: []string{prefixListID},
				},
			},
		})
		if err == nil && len(result.PrefixLists) == 1 {
			return &result.PrefixLists[0], nil
		}
		logger.Info("Failed to get prefix list with ID from annotation, falling back to expected prefix list name")
	}

	prefixListName := fmt.Sprintf("%s-%s-tgw-prefixlist", r.clusterClient.GetManagementClusterNamespacedName().Name, r.clusterClient.GetManagementClusterNamespacedName().Namespace)
	result, err := r.transitGatewayClient.DescribeManagedPrefixLists(ctx, &ec2.DescribeManagedPrefixListsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("prefix-list-name"),
				Values: []string{prefixListName},
			},
		},
	})
	if err != nil {
		logger.Error(err, "Failed to get prefix list", "prefixListName", prefixListName)
		return nil, err
	}

	if len(result.PrefixLists) > 1 {
		return nil, fmt.Errorf("unexpected number of prefix lists returned")
	} else if len(result.PrefixLists) == 1 {
		return &result.PrefixLists[0], nil
	}

	output, err := r.transitGatewayClient.CreateManagedPrefixList(ctx, &ec2.CreateManagedPrefixListInput{
		AddressFamily:  aws.String("IPv4"),
		MaxEntries:     aws.Int32(PREFIX_LIST_MAX_ENTRIES),
		PrefixListName: &prefixListName,
	})
	if err != nil {
		logger.Error(err, "Failed to create prefix list", "prefixListName", prefixListName)
		return nil, err
	}

	logger.Info("Created new prefix list", "prefixListName", prefixListName)
	return output.PrefixList, nil
}

func (r *TransitGateway) addRoutes(ctx context.Context, transitGatewayID, prefixListID *string, awsCluster *capa.AWSCluster) error {
	logger := r.getLogger(ctx)

	subnets := []string{}
	for _, s := range awsCluster.Spec.NetworkSpec.Subnets {
		subnets = append(subnets, s.ID)
	}

	output, err := r.transitGatewayClient.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{Name: aws.String("association.subnet-id"), Values: subnets},
		},
	})
	if err != nil {
		logger.Error(err, "Failed to get route tables")
		return err
	}

	if output != nil && len(output.RouteTables) > 0 {
		for _, rt := range output.RouteTables {
			matchFound := false
			for _, route := range rt.Routes {
				if route.DestinationPrefixListId != nil && route.TransitGatewayId != nil && *route.DestinationPrefixListId == *prefixListID && *route.TransitGatewayId == *transitGatewayID {
					// route already exists
					matchFound = true
				}
			}
			if matchFound {
				continue
			}

			_, err = r.transitGatewayClient.CreateRoute(ctx, &ec2.CreateRouteInput{
				RouteTableId:            rt.RouteTableId,
				DestinationPrefixListId: prefixListID,
				TransitGatewayId:        transitGatewayID,
			})
			if err != nil {
				logger.Error(err, "Failed to add route to route table", "routeTableID", rt.RouteTableId, "prefixListID", prefixListID, "transitGatewayID", transitGatewayID)
				return err
			}
			logger.Info("Added routes to route table", "routeTableID", rt.RouteTableId, "prefixListID", prefixListID, "transitGatewayID", transitGatewayID)
		}
	}

	return nil
}

func (r *TransitGateway) removeRoutes(ctx context.Context, awsCluster *capa.AWSCluster) error {
	logger := r.getLogger(ctx)

	prefixList, err := r.getOrCreatePrefixList(ctx)
	if err != nil {
		return err
	}
	prefixListID := *prefixList.PrefixListId

	err = r.removeFromPrefixList(ctx, awsCluster)
	if err != nil {
		logger.Error(err, "Failed to remove CIDR from prefix list", "prefixListID", prefixListID, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
		return err
	}

	subnets := []string{}
	for _, s := range awsCluster.Spec.NetworkSpec.Subnets {
		subnets = append(subnets, s.ID)
	}

	output, err := r.transitGatewayClient.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{Name: aws.String("association.subnet-id"), Values: subnets},
		},
	})
	if err != nil {
		logger.Error(err, "Failed to get route tables")
		return err
	}

	if output != nil && len(output.RouteTables) > 0 {
		for _, rt := range output.RouteTables {
			_, err = r.transitGatewayClient.DeleteRoute(ctx, &ec2.DeleteRouteInput{
				RouteTableId:            rt.RouteTableId,
				DestinationPrefixListId: &prefixListID,
			})
			if err != nil {
				logger.Error(err, "Failed to remove route from route table", "routeTableID", rt.RouteTableId, "prefixListID", prefixListID)
				return err
			}
			logger.Info("Removed routes from route table", "routeTableID", rt.RouteTableId, "prefixListID", prefixListID)
		}
	}

	return nil
}

func (r *TransitGateway) addToPrefixList(ctx context.Context, awsCluster *capa.AWSCluster) (string, error) {
	logger := r.getLogger(ctx)

	prefixList, err := r.getOrCreatePrefixList(ctx)
	if err != nil {
		return "", err
	}
	prefixListID := *prefixList.PrefixListId

	result, err := r.transitGatewayClient.GetManagedPrefixListEntries(ctx, &ec2.GetManagedPrefixListEntriesInput{
		PrefixListId:  &prefixListID,
		MaxResults:    aws.Int32(100),
		TargetVersion: prefixList.Version,
	})
	if err != nil {
		logger.Error(err, "Failed to get prefix list entries", "prefixListID", prefixListID, "version", prefixList.Version)
		return prefixListID, err
	}

	description := buildEntryDescription(awsCluster)

	for _, entry := range result.Entries {
		if *entry.Cidr == awsCluster.Spec.NetworkSpec.VPC.CidrBlock {
			if *entry.Description != description {
				err = fmt.Errorf("conflicting CIDR already exists on prefix list")
				logger.Error(err, "The CIDR already exists on the prefix list and belongs to another cluster", "prefixListID", prefixListID, "version", prefixList.Version, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
				return prefixListID, err
			}

			// entry already exists
			logger.Info("Entry already exists in prefix list, skipping")
			return prefixListID, err
		}
	}

	_, err = r.transitGatewayClient.ModifyManagedPrefixList(ctx, &ec2.ModifyManagedPrefixListInput{
		PrefixListId:   &prefixListID,
		CurrentVersion: prefixList.Version,
		AddEntries: []types.AddPrefixListEntry{
			{
				Cidr:        &awsCluster.Spec.NetworkSpec.VPC.CidrBlock,
				Description: &description,
			},
		},
	})
	if err != nil {
		logger.Error(err, "Failed to add to prefix list", "prefixListID", prefixListID, "version", prefixList.Version, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
		return prefixListID, err
	}

	logger.Info("Added CIDR to prefix list", "prefixListID", prefixListID, "version", prefixList.Version, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
	return prefixListID, nil
}

func (r *TransitGateway) removeFromPrefixList(ctx context.Context, awsCluster *capa.AWSCluster) error {
	logger := r.getLogger(ctx)

	prefixList, err := r.getOrCreatePrefixList(ctx)
	if err != nil {
		return err
	}

	result, err := r.transitGatewayClient.GetManagedPrefixListEntries(ctx, &ec2.GetManagedPrefixListEntriesInput{
		PrefixListId:  prefixList.PrefixListId,
		MaxResults:    aws.Int32(100),
		TargetVersion: prefixList.Version,
	})
	if err != nil {
		logger.Error(err, "Failed to get prefix list entries", "prefixListID", prefixList.PrefixListId, "version", prefixList.Version)
		return err
	}

	for _, entry := range result.Entries {
		if *entry.Cidr == awsCluster.Spec.NetworkSpec.VPC.CidrBlock && *entry.Description == buildEntryDescription(awsCluster) {
			_, err = r.transitGatewayClient.ModifyManagedPrefixList(ctx, &ec2.ModifyManagedPrefixListInput{
				PrefixListId:   prefixList.PrefixListId,
				CurrentVersion: prefixList.Version,
				RemoveEntries: []types.RemovePrefixListEntry{
					{Cidr: &awsCluster.Spec.NetworkSpec.VPC.CidrBlock},
				},
			})
			if err != nil {
				logger.Error(err, "Failed to remove from prefix list", "prefixListID", prefixList.PrefixListId, "version", prefixList.Version, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
				return err
			}

			logger.Info("Removed CIDR from prefix list", "prefixListID", prefixList.PrefixListId, "version", prefixList.Version, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
			return nil
		}
	}

	return nil
}

func getPrivateSubnetsByAZ(subnets capa.Subnets) map[string]capa.Subnets {
	subnetMap := map[string]capa.Subnets{}

	for _, subnet := range subnets {
		if !subnet.IsPublic {
			if _, ok := subnetMap[subnet.AvailabilityZone]; !ok {
				subnetMap[subnet.AvailabilityZone] = capa.Subnets{}
			}

			subnetMap[subnet.AvailabilityZone] = append(subnetMap[subnet.AvailabilityZone], subnet)
		}
	}

	return subnetMap
}

func buildEntryDescription(awsCluster *capa.AWSCluster) string {
	return fmt.Sprintf("CIDR block for cluster %s", awsCluster.ObjectMeta.Name)
}

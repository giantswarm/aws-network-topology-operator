package registrar

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/giantswarm/k8smetadata/pkg/annotation"
	"github.com/go-logr/logr"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	awsclient "github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
)

type contextKey string

var clusterNameContextKey contextKey = "clusterName"
var tagKey = "tag:"

const (
	// PREFIX_LIST_MAX_ENTRIES is the maximum number of entries a created prefix list can have.
	// This number counts against a resources quota (regardless of how many actual entries exist)
	// when it is referenced. We're setting the max here to 45 for now so we stay below the
	// default "Routes per route table" quota of 50.
	PREFIX_LIST_MAX_ENTRIES = 45

	SubnetTGWAttachementsLabel = "subnet.giantswarm.io/tgw"
	SubnetRoleLabel            = "github.com/giantswarm/aws-vpc-operator/role"

	ErrRouteNotFound = "InvalidRoute.NotFound"
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

	gatewayID, err := getTransitGatewayID(logger, cluster)
	if err != nil {
		return err
	}

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

		prefixListID, err := getPrefixListID(logger, cluster)
		if err != nil {
			return err
		}

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

				gatewayID, err = getTransitGatewayID(logger, mc)
				if err != nil {
					return err
				}
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
		annotations.SetNetworkTopologyTransitGateway(cluster, *tgw.TransitGatewayArn)
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

		if tgwAttachment.State == types.TransitGatewayAttachmentStatePendingAcceptance {
			logger.Info("Sending SNS message")

			_, err = r.transitGatewayClient.PublishSNSMessage(ctx, &sns.PublishInput{
				Message: awssdk.String("Request TransitGatewayAttachment"),
				MessageAttributes: map[string]snstypes.MessageAttributeValue{
					"Postfach":      {DataType: awssdk.String("String"), StringValue: awssdk.String("support@giantswarm.io")},
					"Account_ID":    {DataType: awssdk.String("String"), StringValue: tgwAttachment.VpcOwnerId},
					"Attachment_ID": {DataType: awssdk.String("String"), StringValue: tgwAttachment.TransitGatewayAttachmentId},
					"CIDR":          {DataType: awssdk.String("String"), StringValue: &awsCluster.Spec.NetworkSpec.VPC.CidrBlock},
					"Name":          {DataType: awssdk.String("String"), StringValue: &cluster.ObjectMeta.Name},
				},
			})
			if err != nil {
				logger.Error(err, "Failed sending SNS message")
				return err
			}
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

				gatewayID, err = getTransitGatewayID(logger, mc)
				if err != nil {
					return err
				}
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
		annotations.SetNetworkTopologyTransitGateway(cluster, *tgw.TransitGatewayArn)
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

		prefixList, err := r.addToPrefixList(ctx, awsCluster)
		if err != nil {
			return err
		}
		prefixListID := *prefixList.PrefixListId

		// Ensure PrefixListID is saved back to the current cluster
		baseCluster = cluster.DeepCopy()
		annotations.SetNetworkTopologyPrefixList(cluster, *prefixList.PrefixListArn)
		if _, err = r.clusterClient.Patch(ctx, cluster, client.MergeFrom(baseCluster)); err != nil {
			logger.Error(err, "Failed to patch cluster resource with prefix list ID", "prefixListID", prefixListID)
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

	gatewayID, err := getTransitGatewayID(logger, cluster)
	if err != nil {
		return err
	}

	switch val := annotations.GetAnnotation(cluster, annotation.NetworkTopologyModeAnnotation); val {
	case "":
		fallthrough

	case annotation.NetworkTopologyModeNone:
		logger.Info("Mode currently not handled", "mode", annotation.NetworkTopologyModeNone)

	case annotation.NetworkTopologyModeUserManaged:
		awsCluster, err := r.getAWSCluster(ctx, cluster)
		if k8sErrors.IsNotFound(err) {
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
		if k8sErrors.IsNotFound(err) {
			logger.Info("AWSCluster is already deleted, skipping transit gateway deletion")
			return nil
		} else if err != nil {
			logger.Error(err, "Failed to get AWSCluster for Cluster")
			return err
		}

		if err := r.removeFromPrefixList(ctx, awsCluster); err != nil {
			return err
		}

		if err := r.detachTransitGateway(ctx, &gatewayID, awsCluster); err != nil {
			return err
		}

		if err := r.deleteTransitGateway(ctx, &gatewayID, cluster); err != nil {
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
			Description: awssdk.String(fmt.Sprintf("Transit Gateway for cluster %s", ctx.Value(clusterNameContextKey))),
			Options: &types.TransitGatewayRequestOptions{
				AutoAcceptSharedAttachments: types.AutoAcceptSharedAttachmentsValueEnable,
			},
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeTransitGateway,
					Tags: []types.Tag{
						{
							Key:   awssdk.String(fmt.Sprintf("kubernetes.io/cluster/%s", ctx.Value(clusterNameContextKey))),
							Value: awssdk.String("owned"),
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

	// Attachments from VPC to the transit gateway need to be made from the AWS account
	// of the workload cluster, so we use a separate client
	transitGatewayAttachmentClient := r.getTransitGatewayClientForWorkloadCluster(k8stypes.NamespacedName{
		Name:      awsCluster.ObjectMeta.Name,
		Namespace: awsCluster.ObjectMeta.Namespace,
	})

	vpcID := awsCluster.Spec.NetworkSpec.VPC.ID
	subnets, err := r.getTGWGAttachmentSubnetsOrDefault(ctx, transitGatewayAttachmentClient, awsCluster)
	if err != nil {
		logger.Error(err, "Failed to get subnets for transit gateway attachment", "transitGatewayID", gatewayID)
		return nil, err
	}

	if vpcID == "" || len(subnets) == 0 {
		err := fmt.Errorf("cluster network not yet available on AWSCluster resource")
		logger.Error(err, "AWSCluster does not yet have network details available")
		return nil, err
	}

	describeTGWattachmentInput := &ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []types.Filter{
			{
				Name:   awssdk.String("transit-gateway-id"),
				Values: []string{*gatewayID},
			},
			{
				Name:   awssdk.String("vpc-id"),
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
							Key:   awssdk.String("Name"),
							Value: awssdk.String(awsCluster.Name),
						},
						{
							Key:   awssdk.String(fmt.Sprintf("kubernetes.io/cluster/%s", ctx.Value(clusterNameContextKey))),
							Value: awssdk.String("owned"),
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

func (r *TransitGateway) deleteTransitGateway(ctx context.Context, gatewayID *string, cluster *capi.Cluster) error {
	logger := r.getLogger(ctx)
	logger = logger.WithValues("transitGatewayID", gatewayID)

	logger.Info("Deleting transit gateway")
	defer logger.Info("Done deleting transit gateway")

	if !r.clusterClient.IsManagementCluster(ctx, cluster) {
		logger.Info("Cluster is a workload cluster. Skipping transit gateway deletion")
		return nil
	}

	describeTGWattachmentInput := &ec2.DeleteTransitGatewayInput{
		TransitGatewayId: gatewayID,
	}

	_, err := r.transitGatewayClient.DeleteTransitGateway(ctx, describeTGWattachmentInput)
	if err != nil {
		logger.Error(err, "failed to delete transit gateway")
		return err
	}

	return nil
}

func (r *TransitGateway) detachTransitGateway(ctx context.Context, gatewayID *string, awsCluster *capa.AWSCluster) error {
	logger := r.getLogger(ctx)

	vpcID := awsCluster.Spec.NetworkSpec.VPC.ID
	if vpcID == "" {
		logger.Info("VPC already deleted. Skipping removing transit gateway attachments")
		return nil
	}

	describeTGWattachmentInput := &ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []types.Filter{
			{
				Name:   awssdk.String("transit-gateway-id"),
				Values: []string{*gatewayID},
			},
			{
				Name:   awssdk.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	}

	transitGatewayAttachmentClient := r.getTransitGatewayClientForWorkloadCluster(k8stypes.NamespacedName{
		Name:      awsCluster.Name,
		Namespace: awsCluster.Namespace,
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

	prefixListID, err := getPrefixListID(logger, mc)
	if err != nil {
		logger.Error(err, "Failed to get prefix list id from cluster")
		return nil, err
	}

	if prefixListID != "" {
		result, err := r.transitGatewayClient.DescribeManagedPrefixLists(ctx, &ec2.DescribeManagedPrefixListsInput{
			Filters: []types.Filter{
				{
					Name:   awssdk.String("prefix-list-id"),
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
				Name:   awssdk.String("prefix-list-name"),
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
		AddressFamily:  awssdk.String("IPv4"),
		MaxEntries:     awssdk.Int32(PREFIX_LIST_MAX_ENTRIES),
		PrefixListName: &prefixListName,
	})
	if err != nil {
		logger.Error(err, "Failed to create prefix list", "prefixListName", prefixListName)
		return nil, err
	}

	logger.Info("Created new prefix list", "prefixListName", prefixListName)
	return output.PrefixList, nil
}

func (r *TransitGateway) addToPrefixList(ctx context.Context, awsCluster *capa.AWSCluster) (*types.ManagedPrefixList, error) {
	logger := r.getLogger(ctx)

	prefixList, err := r.getOrCreatePrefixList(ctx)
	if err != nil {
		return nil, err
	}
	prefixListID := *prefixList.PrefixListId

	result, err := r.transitGatewayClient.GetManagedPrefixListEntries(ctx, &ec2.GetManagedPrefixListEntriesInput{
		PrefixListId:  &prefixListID,
		MaxResults:    awssdk.Int32(100),
		TargetVersion: prefixList.Version,
	})
	if err != nil {
		logger.Error(err, "Failed to get prefix list entries", "prefixListID", prefixListID, "version", prefixList.Version)
		return nil, err
	}

	description := buildEntryDescription(awsCluster)

	for _, entry := range result.Entries {
		if *entry.Cidr == awsCluster.Spec.NetworkSpec.VPC.CidrBlock {
			if *entry.Description != description {
				err = fmt.Errorf("conflicting CIDR already exists on prefix list")
				logger.Error(err, "The CIDR already exists on the prefix list and belongs to another cluster", "prefixListID", prefixListID, "version", prefixList.Version, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
				return nil, err
			}

			// entry already exists
			logger.Info("Entry already exists in prefix list, skipping")
			return prefixList, err
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
		return nil, err
	}

	logger.Info("Added CIDR to prefix list", "prefixListID", prefixListID, "version", prefixList.Version, "cidr", awsCluster.Spec.NetworkSpec.VPC.CidrBlock)
	return prefixList, nil
}

func (r *TransitGateway) removeFromPrefixList(ctx context.Context, awsCluster *capa.AWSCluster) error {
	logger := r.getLogger(ctx)

	prefixList, err := r.getOrCreatePrefixList(ctx)
	if err != nil {
		return err
	}

	result, err := r.transitGatewayClient.GetManagedPrefixListEntries(ctx, &ec2.GetManagedPrefixListEntriesInput{
		PrefixListId:  prefixList.PrefixListId,
		MaxResults:    awssdk.Int32(100),
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

// Search subnets with expected attachment, if there are not any
// choose first one per AZ
func (r *TransitGateway) getTGWGAttachmentSubnetsOrDefault(ctx context.Context, transitGatewayClient awsclient.TransitGatewayClient, awsCluster *capa.AWSCluster) ([]string, error) {
	result := make([]string, 0)
	output, err := transitGatewayClient.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{
			{Name: awssdk.String(tagKey + capa.NameKubernetesAWSCloudProviderPrefix + awsCluster.Name), Values: []string{"owned", "shared"}},
			{Name: awssdk.String(tagKey + SubnetTGWAttachementsLabel), Values: []string{"true"}},
			{Name: awssdk.String(tagKey + SubnetRoleLabel), Values: []string{"private"}},
		},
	})
	if err != nil {
		return nil, err
	}

	if output == nil || len(output.Subnets) == 0 {
		result := getPrivateSubnetsByAZ(awsCluster.Spec.NetworkSpec.Subnets)
		return result, nil
	}

	azMap := make(map[string]bool)
	for _, subnet := range output.Subnets {
		if !azMap[*subnet.AvailabilityZone] {
			result = append(result, *subnet.SubnetId)
			azMap[*subnet.AvailabilityZone] = true
		}
	}
	return result, nil
}

func getPrivateSubnetsByAZ(subnets capa.Subnets) []string {
	azMap := make(map[string]bool)
	result := make([]string, 0)
	for _, subnet := range subnets {
		if !subnet.IsPublic {
			if !azMap[subnet.AvailabilityZone] {
				result = append(result, subnet.ID)
				azMap[subnet.AvailabilityZone] = true
			}
		}
	}

	return result
}

func buildEntryDescription(awsCluster *capa.AWSCluster) string {
	return fmt.Sprintf("CIDR block for cluster %s", awsCluster.Name)
}

func getTransitGatewayID(logger logr.Logger, cluster *capi.Cluster) (string, error) {
	gatewayAnnotation := annotations.GetNetworkTopologyTransitGateway(cluster)
	if gatewayAnnotation == "" {
		return "", nil
	}

	// For migration purposes we allow the transit gateway annotation to either
	// contain an ARN or the ID. If we can't parse the arn we assume that an ID
	// is provided. We always save the ARN later
	transitGatewayID, err := aws.GetARNResourceID(gatewayAnnotation)
	if err != nil {
		logger.Info("Failed to parse transit gateway ARN, assuming ID is provided")
		return gatewayAnnotation, nil
	}

	return transitGatewayID, nil
}

func getPrefixListID(logger logr.Logger, cluster *capi.Cluster) (string, error) {
	prefixListAnnoation := annotations.GetNetworkTopologyPrefixList(cluster)
	if prefixListAnnoation == "" {
		return "", nil
	}

	// For migration purposes we allow the prefix list annotation to either
	// contain an ARN or the ID. If we can't parse the arn we assume that an ID
	// is provided. We always save the ARN later
	prefixListID, err := aws.GetARNResourceID(prefixListAnnoation)
	if err != nil {
		logger.Info("Failed to parse prefix list ARN, assuming ID is provided")
		return prefixListAnnoation, nil
	}

	return prefixListID, nil
}

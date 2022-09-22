package aws

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
)

//counterfeiter:generate . TransitGatewayClient
type TransitGatewayClient interface {
	CreateTransitGateway(ctx context.Context, params *ec2.CreateTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayOutput, error)
	DeleteTransitGateway(ctx context.Context, params *ec2.DeleteTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayOutput, error)
	DescribeTransitGateways(ctx context.Context, params *ec2.DescribeTransitGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewaysOutput, error)

	CreateTransitGatewayVpcAttachment(ctx context.Context, params *ec2.CreateTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error)
	DeleteTransitGatewayVpcAttachment(ctx context.Context, params *ec2.DeleteTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error)
	DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error)

	CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error)
	DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	DeleteRoute(ctx context.Context, params *ec2.DeleteRouteInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteOutput, error)

	CreateManagedPrefixList(ctx context.Context, params *ec2.CreateManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.CreateManagedPrefixListOutput, error)
	DescribeManagedPrefixLists(ctx context.Context, params *ec2.DescribeManagedPrefixListsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeManagedPrefixListsOutput, error)
	ModifyManagedPrefixList(ctx context.Context, params *ec2.ModifyManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.ModifyManagedPrefixListOutput, error)
	GetManagedPrefixListEntries(ctx context.Context, params *ec2.GetManagedPrefixListEntriesInput, optFns ...func(*ec2.Options)) (*ec2.GetManagedPrefixListEntriesOutput, error)
}

type EC2Client struct {
	ctx               context.Context
	ec2Client         *ec2.Client
	k8sClient         *k8sclient.Cluster
	managementCluster types.NamespacedName
}

func NewEC2Client(ctx context.Context, k8sClient *k8sclient.Cluster, managementCluster types.NamespacedName) *EC2Client {
	return &EC2Client{
		ctx:               ctx,
		ec2Client:         nil,
		k8sClient:         k8sClient,
		managementCluster: managementCluster,
	}
}

func (e *EC2Client) client() *ec2.Client {
	if e.ec2Client == nil {
		logger := log.FromContext(e.ctx)

		logger.Info("assuming ClusterRoleIdentity role of management cluster")

		identity, err := e.k8sClient.GetAWSClusterRoleIdentity(e.ctx, e.managementCluster)
		if err != nil {
			logger.Error(err, "failed to get ClusterRoleIdentity of management cluster")
			os.Exit(1)
		}
		roleARN := identity.Spec.RoleArn

		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			logger.Error(err, "unable to load AWS SDK config")
			os.Exit(1)
		}

		creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), roleARN)

		cfg, err = config.LoadDefaultConfig(context.TODO(), config.WithCredentialsProvider(aws.NewCredentialsCache(creds)))
		if err != nil {
			logger.Error(err, "unable to assume IAM role")
			os.Exit(1)
		}

		e.ec2Client = ec2.NewFromConfig(cfg)
	}

	return e.ec2Client
}

func (e *EC2Client) CreateTransitGateway(ctx context.Context, params *ec2.CreateTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayOutput, error) {
	return e.client().CreateTransitGateway(ctx, params, optFns...)
}

func (e *EC2Client) CreateTransitGatewayVpcAttachment(ctx context.Context, params *ec2.CreateTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error) {
	return e.client().CreateTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *EC2Client) DeleteTransitGateway(ctx context.Context, params *ec2.DeleteTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayOutput, error) {
	return e.client().DeleteTransitGateway(ctx, params, optFns...)
}

func (e *EC2Client) DeleteTransitGatewayVpcAttachment(ctx context.Context, params *ec2.DeleteTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error) {
	return e.client().DeleteTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *EC2Client) DescribeTransitGateways(ctx context.Context, params *ec2.DescribeTransitGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewaysOutput, error) {
	return e.client().DescribeTransitGateways(ctx, params, optFns...)
}

func (e *EC2Client) DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	return e.client().DescribeTransitGatewayVpcAttachments(ctx, params, optFns...)
}

func (e *EC2Client) CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	return e.client().CreateRoute(ctx, params, optFns...)
}

func (e *EC2Client) DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return e.client().DescribeRouteTables(ctx, params, optFns...)
}

func (e *EC2Client) DeleteRoute(ctx context.Context, params *ec2.DeleteRouteInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteOutput, error) {
	return e.client().DeleteRoute(ctx, params, optFns...)
}

func (e *EC2Client) CreateManagedPrefixList(ctx context.Context, params *ec2.CreateManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.CreateManagedPrefixListOutput, error) {
	return e.client().CreateManagedPrefixList(ctx, params, optFns...)
}

func (e *EC2Client) DescribeManagedPrefixLists(ctx context.Context, params *ec2.DescribeManagedPrefixListsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeManagedPrefixListsOutput, error) {
	return e.client().DescribeManagedPrefixLists(ctx, params, optFns...)
}

func (e *EC2Client) ModifyManagedPrefixList(ctx context.Context, params *ec2.ModifyManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.ModifyManagedPrefixListOutput, error) {
	return e.client().ModifyManagedPrefixList(ctx, params, optFns...)
}

func (e *EC2Client) GetManagedPrefixListEntries(ctx context.Context, params *ec2.GetManagedPrefixListEntriesInput, optFns ...func(*ec2.Options)) (*ec2.GetManagedPrefixListEntriesOutput, error) {
	return e.client().GetManagedPrefixListEntries(ctx, params, optFns...)
}

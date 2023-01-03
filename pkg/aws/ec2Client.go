package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
)

type EC2Client struct {
	ctx       context.Context
	ec2Client *ec2.Client
	k8sClient *k8sclient.Cluster
	cluster   types.NamespacedName
}

func NewEC2Client(ctx context.Context, k8sClient *k8sclient.Cluster, cluster types.NamespacedName) *EC2Client {
	return &EC2Client{
		ctx:       ctx,
		ec2Client: nil,
		k8sClient: k8sClient,
		cluster:   cluster,
	}
}

func (e *EC2Client) client() (*ec2.Client, error) {
	if e.ec2Client == nil {
		logger := log.FromContext(e.ctx)

		logger.Info("assuming ClusterRoleIdentity role of cluster", "cluster", e.cluster.Name)

		identity, err := e.k8sClient.GetAWSClusterRoleIdentity(e.ctx, e.cluster)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get ClusterRoleIdentity of cluster %s", e.cluster.Name)
		}
		roleARN := identity.Spec.RoleArn

		cfg, err := config.LoadDefaultConfig(e.ctx)
		if err != nil {
			return nil, errors.Wrap(err, "unable to load AWS SDK config")
		}

		creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), roleARN)

		cfg, err = config.LoadDefaultConfig(e.ctx, config.WithCredentialsProvider(aws.NewCredentialsCache(creds)))
		if err != nil {
			return nil, errors.Wrapf(err, "unable to assume IAM role %s", roleARN)
		}

		e.ec2Client = ec2.NewFromConfig(cfg)
	}

	return e.ec2Client, nil
}

func (e *EC2Client) CreateTransitGateway(ctx context.Context, params *ec2.CreateTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.CreateTransitGateway(ctx, params, optFns...)
}

func (e *EC2Client) CreateTransitGatewayVpcAttachment(ctx context.Context, params *ec2.CreateTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.CreateTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *EC2Client) DeleteTransitGateway(ctx context.Context, params *ec2.DeleteTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.DeleteTransitGateway(ctx, params, optFns...)
}

func (e *EC2Client) DeleteTransitGatewayVpcAttachment(ctx context.Context, params *ec2.DeleteTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.DeleteTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *EC2Client) DescribeTransitGateways(ctx context.Context, params *ec2.DescribeTransitGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewaysOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.DescribeTransitGateways(ctx, params, optFns...)
}

func (e *EC2Client) DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.DescribeTransitGatewayVpcAttachments(ctx, params, optFns...)
}

func (e *EC2Client) CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.CreateRoute(ctx, params, optFns...)
}

func (e *EC2Client) DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.DescribeRouteTables(ctx, params, optFns...)
}

func (e *EC2Client) DeleteRoute(ctx context.Context, params *ec2.DeleteRouteInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.DeleteRoute(ctx, params, optFns...)
}

func (e *EC2Client) CreateManagedPrefixList(ctx context.Context, params *ec2.CreateManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.CreateManagedPrefixListOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.CreateManagedPrefixList(ctx, params, optFns...)
}

func (e *EC2Client) DescribeManagedPrefixLists(ctx context.Context, params *ec2.DescribeManagedPrefixListsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeManagedPrefixListsOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.DescribeManagedPrefixLists(ctx, params, optFns...)
}

func (e *EC2Client) ModifyManagedPrefixList(ctx context.Context, params *ec2.ModifyManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.ModifyManagedPrefixListOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.ModifyManagedPrefixList(ctx, params, optFns...)
}

func (e *EC2Client) GetManagedPrefixListEntries(ctx context.Context, params *ec2.GetManagedPrefixListEntriesInput, optFns ...func(*ec2.Options)) (*ec2.GetManagedPrefixListEntriesOutput, error) {
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	return client.GetManagedPrefixListEntries(ctx, params, optFns...)
}

package aws

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

//counterfeiter:generate . TransitGatewayClient
type TransitGatewayClient interface {
	CreateTransitGateway(ctx context.Context, params *ec2.CreateTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayOutput, error)
	CreateTransitGatewayVpcAttachment(ctx context.Context, params *ec2.CreateTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error)
	DeleteTransitGateway(ctx context.Context, params *ec2.DeleteTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayOutput, error)
	DeleteTransitGatewayVpcAttachment(ctx context.Context, params *ec2.DeleteTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error)
	DescribeTransitGateways(ctx context.Context, params *ec2.DescribeTransitGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewaysOutput, error)
	DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error)
}

type EC2Client struct {
	ec2Client *ec2.Client
}

func NewEC2Client(ctx context.Context, roleARN string) *EC2Client {
	logger := log.FromContext(ctx)

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

	return &EC2Client{
		ec2Client: ec2.NewFromConfig(cfg),
	}
}

func (e *EC2Client) CreateTransitGateway(ctx context.Context, params *ec2.CreateTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayOutput, error) {
	return e.ec2Client.CreateTransitGateway(ctx, params, optFns...)
}

func (e *EC2Client) CreateTransitGatewayVpcAttachment(ctx context.Context, params *ec2.CreateTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error) {
	return e.ec2Client.CreateTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *EC2Client) DeleteTransitGateway(ctx context.Context, params *ec2.DeleteTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayOutput, error) {
	return e.ec2Client.DeleteTransitGateway(ctx, params, optFns...)
}

func (e *EC2Client) DeleteTransitGatewayVpcAttachment(ctx context.Context, params *ec2.DeleteTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error) {
	return e.ec2Client.DeleteTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *EC2Client) DescribeTransitGateways(ctx context.Context, params *ec2.DescribeTransitGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewaysOutput, error) {
	return e.ec2Client.DescribeTransitGateways(ctx, params, optFns...)
}

func (e *EC2Client) DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	return e.ec2Client.DescribeTransitGatewayVpcAttachments(ctx, params, optFns...)
}

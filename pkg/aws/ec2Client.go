package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
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

func NewEC2Client(ec2Service *ec2.Client) *EC2Client {
	return &EC2Client{
		ec2Client: ec2Service,
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

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sns"
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

	PublishSNSMessage(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)

	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
}

type TGWClient struct {
	ec2Client EC2Client
	snsClient SNSClient
}

func NewTGWClient(ec2Client EC2Client, snsClient SNSClient) *TGWClient {
	return &TGWClient{
		ec2Client: ec2Client,
		snsClient: snsClient,
	}
}

func (e *TGWClient) CreateTransitGateway(ctx context.Context, params *ec2.CreateTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayOutput, error) {
	return e.ec2Client.CreateTransitGateway(ctx, params, optFns...)
}

func (e *TGWClient) CreateTransitGatewayVpcAttachment(ctx context.Context, params *ec2.CreateTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error) {
	return e.ec2Client.CreateTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *TGWClient) DeleteTransitGateway(ctx context.Context, params *ec2.DeleteTransitGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayOutput, error) {
	return e.ec2Client.DeleteTransitGateway(ctx, params, optFns...)
}

func (e *TGWClient) DeleteTransitGatewayVpcAttachment(ctx context.Context, params *ec2.DeleteTransitGatewayVpcAttachmentInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error) {
	return e.ec2Client.DeleteTransitGatewayVpcAttachment(ctx, params, optFns...)
}

func (e *TGWClient) DescribeTransitGateways(ctx context.Context, params *ec2.DescribeTransitGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewaysOutput, error) {
	return e.ec2Client.DescribeTransitGateways(ctx, params, optFns...)
}

func (e *TGWClient) DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	return e.ec2Client.DescribeTransitGatewayVpcAttachments(ctx, params, optFns...)
}

func (e *TGWClient) CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	return e.ec2Client.CreateRoute(ctx, params, optFns...)
}

func (e *TGWClient) DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return e.ec2Client.DescribeRouteTables(ctx, params, optFns...)
}

func (e *TGWClient) DeleteRoute(ctx context.Context, params *ec2.DeleteRouteInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteOutput, error) {
	return e.ec2Client.DeleteRoute(ctx, params, optFns...)
}

func (e *TGWClient) CreateManagedPrefixList(ctx context.Context, params *ec2.CreateManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.CreateManagedPrefixListOutput, error) {
	return e.ec2Client.CreateManagedPrefixList(ctx, params, optFns...)
}

func (e *TGWClient) DescribeManagedPrefixLists(ctx context.Context, params *ec2.DescribeManagedPrefixListsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeManagedPrefixListsOutput, error) {
	return e.ec2Client.DescribeManagedPrefixLists(ctx, params, optFns...)
}

func (e *TGWClient) ModifyManagedPrefixList(ctx context.Context, params *ec2.ModifyManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.ModifyManagedPrefixListOutput, error) {
	return e.ec2Client.ModifyManagedPrefixList(ctx, params, optFns...)
}

func (e *TGWClient) GetManagedPrefixListEntries(ctx context.Context, params *ec2.GetManagedPrefixListEntriesInput, optFns ...func(*ec2.Options)) (*ec2.GetManagedPrefixListEntriesOutput, error) {
	return e.ec2Client.GetManagedPrefixListEntries(ctx, params, optFns...)
}

func (e *TGWClient) PublishSNSMessage(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	return e.snsClient.PublishSNSMessage(ctx, params, optFns...)
}

func (e *TGWClient) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {

	return e.ec2Client.DescribeSubnets(ctx, params, optFns...)
}

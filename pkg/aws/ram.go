package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ram"
)

type ResourceShare struct {
	Name              string
	ResourceArns      []string
	ExternalAccountID string
}

type RAMClient struct {
	ramClient *ram.RAM
}

func NewRAMClient(ramClient *ram.RAM) *RAMClient {
	return &RAMClient{ramClient}
}

func (c *RAMClient) ApplyResourceShare(ctx context.Context, share ResourceShare) error {
	_, err := c.ramClient.CreateResourceShare(&ram.CreateResourceShareInput{
		AllowExternalPrincipals: awssdk.Bool(true),
		Name:                    awssdk.String(share.Name),
		Principals:              []*string{awssdk.String(share.ExternalAccountID)},
		ResourceArns:            awssdk.StringSlice(share.ResourceArns),
	})
	return err
}

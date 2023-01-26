package aws

import (
	"context"

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
	return nil
}

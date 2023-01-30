package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const ResourceOwnerSelf = "SELF"

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
	logger := c.getLogger(ctx, share)

	resourceShare, err := c.ramClient.GetResourceShares(&ram.GetResourceSharesInput{
		Name:          aws.String(share.Name),
		ResourceOwner: aws.String(ResourceOwnerSelf),
	})
	if err != nil {
		logger.Error(err, "failed to get resource share")
		return errors.WithStack(err)
	}

	if len(resourceShare.ResourceShares) != 0 {
		logger.Info("resource share already exists")
		return nil
	}

	logger.Info("creating resource share")
	_, err = c.ramClient.CreateResourceShare(&ram.CreateResourceShareInput{
		AllowExternalPrincipals: awssdk.Bool(true),
		Name:                    awssdk.String(share.Name),
		Principals:              []*string{awssdk.String(share.ExternalAccountID)},
		ResourceArns:            awssdk.StringSlice(share.ResourceArns),
	})

	if err != nil {
		logger.Error(err, "failed to create resource share")
		return err
	}

	return nil
}

func (c *RAMClient) getLogger(ctx context.Context, share ResourceShare) logr.Logger {
	logger := log.FromContext(ctx)
	logger = logger.WithName("ram-client")
	logger = logger.WithValues("resource-share-name", share.Name, "resource-arns", share.ResourceArns)
	return logger
}

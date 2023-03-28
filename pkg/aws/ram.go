package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
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
	logger := c.getLogger(ctx)
	logger = logger.WithValues("resource-share-name", share.Name, "resource-arns", share.ResourceArns)

	resourceShare, err := c.getResourceShare(ctx, share.Name)
	if err != nil {
		logger.Error(err, "failed to get resource share")
		return errors.WithStack(err)
	}

	if resourceShare != nil {
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

func (c *RAMClient) DeleteResourceShare(ctx context.Context, name string) error {
	logger := c.getLogger(ctx)
	logger = logger.WithValues("resource-share-name", name)

	resourceShare, err := c.getResourceShare(ctx, name)
	if err != nil {
		logger.Error(err, "failed to get resource share")
		return err
	}

	if resourceShare == nil {
		logger.Info("resource share not found")
		return nil
	}

	_, err = c.ramClient.DeleteResourceShare(&ram.DeleteResourceShareInput{
		ResourceShareArn: resourceShare.ResourceShareArn,
	})
	return err
}

func (c *RAMClient) getResourceShare(ctx context.Context, name string) (*ram.ResourceShare, error) {
	logger := c.getLogger(ctx)
	logger = logger.WithValues("resource-share-name", name)

	resourceShareOutput, err := c.ramClient.GetResourceShares(&ram.GetResourceSharesInput{
		Name:          awssdk.String(name),
		ResourceOwner: awssdk.String(ResourceOwnerSelf),
	})
	if err != nil {
		logger.Error(err, "failed to get resource share")
		return nil, errors.WithStack(err)
	}

	resourceShares := filterDeletedResourceShares(resourceShareOutput.ResourceShares)

	if len(resourceShares) == 0 {
		logger.Info("no resource shares found")
		return nil, nil
	}

	if len(resourceShares) > 1 {
		err = fmt.Errorf("expected 1 resource share, found %d", len(resourceShares))
		logger.Error(err, "wrong number of resource shares")
		return nil, err
	}

	return resourceShares[0], nil
}

func (c *RAMClient) getLogger(ctx context.Context) logr.Logger {
	logger := log.FromContext(ctx)
	logger = logger.WithName("ram-client")
	return logger
}

func AwsRamClientFromARN(sess *session.Session, roleARN, externalID string) *ram.RAM {
	return ram.New(sess, &awssdk.Config{Credentials: stscreds.NewCredentials(sess, roleARN, configureExternalId(roleARN, externalID))})
}

func configureExternalId(roleArn, externalId string) func(provider *stscreds.AssumeRoleProvider) {
	return func(assumeRoleProvider *stscreds.AssumeRoleProvider) {
		if roleArn != "" {
			assumeRoleProvider.RoleARN = roleArn
		}
		if externalId != "" {
			assumeRoleProvider.ExternalID = awssdk.String(externalId)
		}
	}
}

func filterDeletedResourceShares(resourceShares []*ram.ResourceShare) []*ram.ResourceShare {
	filtered := []*ram.ResourceShare{}
	for _, share := range resourceShares {
		if !isResourceShareDeleted(share) {
			filtered = append(filtered, share)
		}
	}

	return filtered
}

func isResourceShareDeleted(resourceShare *ram.ResourceShare) bool {
	if resourceShare.Status == nil {
		return false
	}

	status := *resourceShare.Status
	return status == ram.ResourceShareStatusDeleted || status == ram.ResourceShareStatusDeleting
}

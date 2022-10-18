package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
)

type SNSClient struct {
	ctx               context.Context
	snsClient         *sns.Client
	snsTopic          string
	k8sClient         *k8sclient.Cluster
	managementCluster types.NamespacedName
}

func NewSNSClient(ctx context.Context, snsTopic string, k8sClient *k8sclient.Cluster, managementCluster types.NamespacedName) *SNSClient {
	return &SNSClient{
		ctx:               ctx,
		snsClient:         nil,
		snsTopic:          snsTopic,
		k8sClient:         k8sClient,
		managementCluster: managementCluster,
	}
}

func (s *SNSClient) client() *sns.Client {
	if s.snsClient == nil {
		logger := log.FromContext(s.ctx)

		logger.Info("assuming ClusterRoleIdentity role of management cluster")

		identity, err := s.k8sClient.GetAWSClusterRoleIdentity(s.ctx, s.managementCluster)
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

		s.snsClient = sns.NewFromConfig(cfg)
	}

	return s.snsClient
}

func (s *SNSClient) PublishSNSMessage(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	params.TopicArn = &s.snsTopic
	if params.TopicArn == nil || *params.TopicArn == "" {
		return nil, fmt.Errorf("no SNS topic provided")
	}

	return s.client().Publish(ctx, params, optFns...)
}

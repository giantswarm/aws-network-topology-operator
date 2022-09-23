[![CircleCI](https://circleci.com/gh/giantswarm/aws-network-topology-operator.svg?style=shield)](https://circleci.com/gh/giantswarm/aws-network-topology-operator)

# aws-network-topology-operator

Handles the setup / configuration of high-level AWS networking to allow cross-VPC communication between clusters

## Setup

```shell
./manager
    --leader-elect
    --management-cluster-name my-mc
    --management-cluster-namespace org-giantswarm
```

## Required IAM permissions

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "ec2:CreateTags",
                "ec2:DeleteTags",
                "ec2:DescribeTransitGateways",
                "ec2:DescribeTransitGatewayVpcAttachments",
                "ec2:CreateTransitGateway",
                "ec2:CreateTransitGatewayVpcAttachment",
                "ec2:DeleteTransitGateway",
                "ec2:DeleteTransitGatewayVpcAttachment",
                "ec2:CreateManagedPrefixList",
                "ec2:DescribeManagedPrefixLists",
                "ec2:ModifyManagedPrefixList",
                "ec2:GetManagedPrefixListEntries",
                "ec2:DeleteRoute",
                "ec2:CreateRoute",
                "ec2:DescribeRouteTables"
            ],
            "Resource": "*"
        }
    ]
}
```

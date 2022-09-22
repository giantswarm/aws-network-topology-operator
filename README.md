[![CircleCI](https://circleci.com/gh/giantswarm/aws-network-topology-operator.svg?style=shield)](https://circleci.com/gh/giantswarm/aws-network-topology-operator)

# aws-network-topology-operator

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
                "ec2:ModifyManagedPrefixList"
            ],
            "Resource": "*"
        }
    ]
}
```

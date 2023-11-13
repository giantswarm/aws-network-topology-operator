package conditions

import (
	gsannotation "github.com/giantswarm/k8smetadata/pkg/annotation"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	capiconditions "sigs.k8s.io/cluster-api/util/conditions"

	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
)

const (
	NetworkTopologyCondition capi.ConditionType = "NetworkTopologyReady"
	TransitGatewayCreated    capi.ConditionType = "TransitGatewayCreated"
)

func MarkReady(setter capiconditions.Setter, condition capi.ConditionType) {
	capiconditions.MarkTrue(setter, condition)
}

func MarkModeNotSupported(cluster *capi.Cluster) {
	capiconditions.MarkFalse(cluster, NetworkTopologyCondition,
		"ModeNotSupported", capi.ConditionSeverityInfo,
		"The provided mode '%s' is not supported",
		annotations.GetAnnotation(cluster, gsannotation.NetworkTopologyModeAnnotation),
	)
}

func MarkVPCNotReady(cluster *capi.Cluster) {
	capiconditions.MarkFalse(cluster, NetworkTopologyCondition,
		"VPCNotReady",
		capi.ConditionSeverityInfo,
		"The cluster's VPC is not yet ready",
	)
}

func MarkIDNotProvided(cluster *capi.Cluster, id string) {
	capiconditions.MarkFalse(cluster, NetworkTopologyCondition,
		"RequiredIDMissing",
		capi.ConditionSeverityError,
		"The %s ID is missing from the annotations", id,
	)
}

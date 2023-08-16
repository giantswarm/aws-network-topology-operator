package conditions

import (
	gsannotation "github.com/giantswarm/k8smetadata/pkg/annotation"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	capiconditions "sigs.k8s.io/cluster-api/util/conditions"

	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
)

const networkTopologyCondition capi.ConditionType = "NetworkTopologyReady"

func MarkReady(cluster *capi.Cluster) {
	capiconditions.MarkTrue(cluster, networkTopologyCondition)
}

func MarkModeNotSupported(cluster *capi.Cluster) {
	capiconditions.MarkFalse(cluster, networkTopologyCondition,
		"ModeNotSupported", capi.ConditionSeverityInfo,
		"The provided mode '%s' is not supported",
		annotations.GetAnnotation(cluster, gsannotation.NetworkTopologyModeAnnotation),
	)
}

func MarkVPCNotReady(cluster *capi.Cluster) {
	capiconditions.MarkFalse(cluster, networkTopologyCondition,
		"VPCNotReady",
		capi.ConditionSeverityInfo,
		"The cluster's VPC is not yet ready",
	)
}

func MarkIDNotProvided(cluster *capi.Cluster, id string) {
	capiconditions.MarkFalse(cluster, networkTopologyCondition,
		"RequiredIDMissing",
		capi.ConditionSeverityError,
		"The %s ID is missing from the annotations", id,
	)
}

package annotations

import (
	gsannotation "github.com/giantswarm/k8smetadata/pkg/annotation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func HasNetworkTopologyMode(o metav1.Object) bool {
	return hasAnnotation(o, gsannotation.NetworkTopologyModeAnnotation)
}

func IsNetworkTopologyModeGiantSwarmManaged(o metav1.Object) bool {
	return GetAnnotation(o, gsannotation.NetworkTopologyModeAnnotation) == gsannotation.NetworkTopologyModeGiantSwarmManaged
}

func IsNetworkTopologyModeUserManaged(o metav1.Object) bool {
	return GetAnnotation(o, gsannotation.NetworkTopologyModeAnnotation) == gsannotation.NetworkTopologyModeUserManaged
}

func IsNetworkTopologyModeNone(o metav1.Object) bool {
	return GetAnnotation(o, gsannotation.NetworkTopologyModeAnnotation) == gsannotation.NetworkTopologyModeNone
}

func GetNetworkTopologyTransitGateway(o metav1.Object) string {
	return GetAnnotation(o, gsannotation.NetworkTopologyTransitGatewayIDAnnotation)
}

func GetNetworkTopologyPrefixList(o metav1.Object) string {
	return GetAnnotation(o, gsannotation.NetworkTopologyPrefixListIDAnnotation)
}

func SetNetworkTopologyTransitGateway(o metav1.Object, transitGatewayID string) {
	AddAnnotations(o, map[string]string{
		gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
	})
}

func SetNetworkTopologyPrefixList(o metav1.Object, prefixList string) {
	AddAnnotations(o, map[string]string{
		gsannotation.NetworkTopologyPrefixListIDAnnotation: prefixList,
	})
}

// GetAnnotation returns the value of the specified annotation.
func GetAnnotation(o metav1.Object, annotation string) string {
	annotations := o.GetAnnotations()
	if annotations == nil {
		return ""
	}
	return annotations[annotation]
}

// AddAnnotations sets the desired annotations on the object
func AddAnnotations(o metav1.Object, desired map[string]string) {
	if len(desired) == 0 {
		return
	}
	annotations := o.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
		o.SetAnnotations(annotations)
	}
	for k, v := range desired {
		if cur, ok := annotations[k]; !ok || cur != v {
			annotations[k] = v
		}
	}
}

// hasAnnotation returns true if the object has the specified annotation.
func hasAnnotation(o metav1.Object, annotation string) bool {
	annotations := o.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, ok := annotations[annotation]
	return ok
}

package annotations

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func HasNetworkTopologyMode(o metav1.Object) bool {
	return hasAnnotation(o, NetworkTopologyModeAnnotation)
}

func IsNetworkTopologyModeGiantSwarmManaged(o metav1.Object) bool {
	return GetAnnotation(o, NetworkTopologyModeAnnotation) == NetworkTopologyModeGiantSwarmManaged
}

func IsNetworkTopologyModeUserManaged(o metav1.Object) bool {
	return GetAnnotation(o, NetworkTopologyModeAnnotation) == NetworkTopologyModeUserManaged
}

func IsNetworkTopologyModeNone(o metav1.Object) bool {
	return GetAnnotation(o, NetworkTopologyModeAnnotation) == NetworkTopologyModeNone
}

func GetNetworkTopologyTransitGatewayID(o metav1.Object) string {
	return GetAnnotation(o, NetworkTopologyTransitGatewayIDAnnotation)
}

func GetNetworkTopologyPrefixListID(o metav1.Object) string {
	return GetAnnotation(o, NetworkTopologyPrefixListIDAnnotation)
}

func SetNetworkTopologyTransitGatewayID(o metav1.Object, transitGatewayID string) {
	AddAnnotations(o, map[string]string{
		NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
	})
}

func SetNetworkTopologyPrefixListID(o metav1.Object, prefixListID string) {
	AddAnnotations(o, map[string]string{
		NetworkTopologyPrefixListIDAnnotation: prefixListID,
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

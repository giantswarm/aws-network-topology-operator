package k8sclient

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func GetNamespacedName(o metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Name:      o.GetName(),
		Namespace: o.GetNamespace(),
	}
}

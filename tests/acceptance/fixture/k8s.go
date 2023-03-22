package fixture

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DeleteKubernetesObject(k8sClient client.Client, object client.Object) error {
	if object == nil {
		return nil
	}

	ctx := context.Background()
	err := k8sClient.Delete(ctx, object)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

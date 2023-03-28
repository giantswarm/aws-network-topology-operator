package aws

import (
	"github.com/aws/smithy-go"
	"github.com/pkg/errors"
)

const (
	ErrAssociationNotFound = "InvalidAssociationID.NotFound"
	ErrRouteTableNotFound  = "InvalidRouteTableID.NotFound"
	ErrSubnetNotFound      = "InvalidSubnetID.NotFound"
	ErrVPCNotFound         = "InvalidVpcID.NotFound"
)

func HasErrorCode(err error, code string) bool {
	var apiError smithy.APIError
	ok := errors.As(err, &apiError)
	if !ok {
		return false
	}

	if apiError.ErrorCode() == code {
		return true
	}
	return false
}

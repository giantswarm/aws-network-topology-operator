package registrar

import (
	"errors"
	"fmt"
	"reflect"
)

type ModeNotSupportedError struct {
	err  string
	Mode string
}

func (e *ModeNotSupportedError) Error() string {
	return fmt.Sprintf("mode %s not yet supported: %s", e.Mode, e.err)
}

func (e *ModeNotSupportedError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

func IsModeNotSupportedError(err error) bool {
	return errors.Is(err, &ModeNotSupportedError{})
}

type TransitGatewayNotAvailableError struct{}

func (e *TransitGatewayNotAvailableError) Error() string {
	return "transit gateway not available"
}

func (e *TransitGatewayNotAvailableError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

func IsTransitGatewayNotAvailableError(err error) bool {
	return errors.Is(err, &TransitGatewayNotAvailableError{})
}

type VPCNotReadyError struct{}

func (e *VPCNotReadyError) Error() string {
	return "transit gateway not available"
}

func (e *VPCNotReadyError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

func IsVPCNotReadyError(err error) bool {
	return errors.Is(err, &VPCNotReadyError{})
}

type IDNotProvidedError struct {
	error
	ID string
}

func (e *IDNotProvidedError) Error() string {
	return fmt.Sprintf("%s ID not provided: %s", e.ID, e.error.Error())
}

func (e *IDNotProvidedError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

func IsIDNotProvidedError(err error) bool {
	return errors.Is(err, &IDNotProvidedError{})
}

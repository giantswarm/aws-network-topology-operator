package registrar

import (
	"fmt"
	"reflect"
)

type ModeNotSupportedError struct {
	error
	err  string
	Mode string
}

func (e *ModeNotSupportedError) Error() string {
	return fmt.Sprintf("mode %s not yet supported: %s", e.Mode, e.err)
}

func (e *ModeNotSupportedError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

type TransitGatewayNotAvailableError struct {
	error
}

func (e *TransitGatewayNotAvailableError) Error() string {
	return "transit gateway not available"
}

func (e *TransitGatewayNotAvailableError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

type VPCNotReadyError struct {
	error
}

func (e *VPCNotReadyError) Error() string {
	return "transit gateway not available"
}

func (e *VPCNotReadyError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

type IDNotProvidedError struct {
	error
	ID string
}

func (e *IDNotProvidedError) Error() string {
	return fmt.Sprintf("%s ID not provided: %s", e.ID, e.Error())
}

func (e *IDNotProvidedError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(e)
}

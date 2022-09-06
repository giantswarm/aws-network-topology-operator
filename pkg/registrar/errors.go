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
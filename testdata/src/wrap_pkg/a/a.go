package a

import (
	"fmt"
)

func A() error { // want A:"naked"
	err := fmt.Errorf("foo")
	return err // want `error returned from external package is not wrapped`
}

package a

import (
	"fmt"
)

type Aer interface {
	A() error
}

type As struct{}

func (as *As) A() error { // want A:"naked"
	err := fmt.Errorf("foo")
	return err // want `error returned from external package is not wrapped`
}

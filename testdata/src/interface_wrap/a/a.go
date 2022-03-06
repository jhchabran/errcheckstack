package a

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

type Aer interface {
	A() error
}

type As struct{}

func (as *As) A() error { // want A:"wrapped"
	err := fmt.Errorf("foo")
	return errors.WithStack(err)
}

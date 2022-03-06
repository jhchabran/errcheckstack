package b

import (
	"fmt"

	"github.com/cockroachdb/errors"

	"interface_wrap/a"
)

func B(aer a.Aer) error { // want B:"wrapped"
	err := aer.A()
	return errors.WithStack(err)
}

func Canary() error { // want Canary:"naked"
	return fmt.Errorf("canary") // want `error returned from external package is not wrapped`
}

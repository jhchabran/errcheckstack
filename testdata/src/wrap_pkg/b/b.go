package b

import (
	"fmt"
	"wrap_pkg/a"

	"github.com/cockroachdb/errors"
)

func B() error { // want B:"wrapped"
	err := a.A()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func Canary() error { // want Canary:"naked"
	return fmt.Errorf("canary") // want `error returned from external package is not wrapped`
}

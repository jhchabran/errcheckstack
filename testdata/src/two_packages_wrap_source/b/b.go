package b

import (
	"fmt"
	"two_packages_wrap_source/a"

	"github.com/cockroachdb/errors"
)

func B() error {
	err := a.A()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func Canary() error {
	return fmt.Errorf("canary") // want `error returned from external package is unwrapped`
}

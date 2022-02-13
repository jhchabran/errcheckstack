package b

import (
	"two_packages_wrap_source/a"

	"github.com/cockroachdb/errors"
)

func B() error {
	err := a.A()
	if err != nil {
		return errors.WithStack(err)
	}
}

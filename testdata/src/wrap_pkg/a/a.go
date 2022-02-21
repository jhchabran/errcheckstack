package a

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

func A() error {
	err := fmt.Errorf("foo")
	return errors.WithStack(err)
}

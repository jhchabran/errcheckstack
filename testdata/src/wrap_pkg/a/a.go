package a

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

func A() error { // want A:"wrapped"
	err := fmt.Errorf("foo")
	return errors.WithStack(err)
}

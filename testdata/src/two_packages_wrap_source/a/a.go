package a

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

func A() error { // want A:"wrapped"
	err := fmt.Errorf("foo")
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

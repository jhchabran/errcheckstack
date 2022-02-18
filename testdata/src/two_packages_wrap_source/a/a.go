package a

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

func A() error {
	err := fmt.Errorf("foo")
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

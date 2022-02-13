package a

import "errors"

func A() error {
	return errors.New("foo") // want `error returned from external package is unwrapped`
}

package a

import "errors"

func A() error { // want A:"unwrapped"
	return errors.New("foo") // want `error returned from external package is not wrapped`
}

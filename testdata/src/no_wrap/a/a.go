package a

import "errors"

func A() error { // want A:"naked"
	return errors.New("foo") // want `error returned from external package is not wrapped`
}

package a

import "errors"

func A() error {
	return errors.New("foo")
}

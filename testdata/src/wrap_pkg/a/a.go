package a

import (
	"fmt"
)

func A() error { // want A:"unwrapped"
	err := fmt.Errorf("foo")
	return err
}

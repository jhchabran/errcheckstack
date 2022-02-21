package b

import "no_wrap/a"

func B() error {
	return a.A() // want `error returned from external package is unwrapped`
}

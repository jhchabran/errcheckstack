package b

import "two_packages_wrap_pkg/a"

func B() error {
	return a.A() // want `error returned from external package is unwrapped`
}

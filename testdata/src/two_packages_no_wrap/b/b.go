package b

import "two_packages_no_wrap/a"

func B() error {
	return a.A()
}

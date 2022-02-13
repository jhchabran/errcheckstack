package main

import "two_packages_wrap_pkg/b"

func main() {
	b.B()
	b.Canary()
}

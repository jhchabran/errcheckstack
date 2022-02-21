package main

import (
	"encoding/json"
	"log"

	"github.com/cockroachdb/errors"
)

func main() {
	err := do()
	if err != nil {
		log.Fatal(err)
	}
}

func do() error { // want do:"wrapped"
	_, err := json.Marshal(struct{}{})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

package main

import (
	"github.com/axsmak/aim/internal/cli"
	"github.com/axsmak/aim/internal/errs"
)

var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		errs.Fatal(err.Error())
	}
}

package main

import (
	"github.com/alexflint/go-arg"
	"go-find-version/utils"
	"go-find-version/web"
)

func main() {
	var args utils.Args
	arg.MustParse(&args)
	webEnabled := !args.DisableWeb

	if webEnabled {
		web.Init(args.Port)
	}
}

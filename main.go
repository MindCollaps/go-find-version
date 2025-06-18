package main

import (
	"github.com/alexflint/go-arg"
	"go-find-version/engine"
	"go-find-version/utils"
	"go-find-version/web"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var args utils.Args
	arg.MustParse(&args)
	webEnabled := !args.DisableWeb

	if webEnabled {
		web.Init(args.Port)
	}

	engine.Run(args)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

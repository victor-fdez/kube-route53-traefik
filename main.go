package main

import (
	"flag"

	"go.uber.org/zap"

	"github.com/victor-fdez/kube-route53-traefik/watch"
)

var dryRun bool
var isDev bool
var sLog *zap.SugaredLogger

func main() {
	var log *zap.Logger
	var err error
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	flag.BoolVar(&dryRun, "dry-run", false, "do not update route53 when setting this flag")
	flag.BoolVar(&isDev, "is-dev", false, "log output to console if in development mode")
	flag.Parse()
	if isDev {
		log, err = zap.NewDevelopment()
	} else {
		log, err = zap.NewProduction()
	}
	if err != nil {
		sLog.Panic(err)
	}
	log.Info("Starting kubernetes / route53 / traefik synchronization service")
	if dryRun {
		log.Info("Running in DRYRUN mode")
	}
	sLog = log.Sugar()
	watch.Setup(kubeconfig, dryRun, sLog)
	watch.Start()
}

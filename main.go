package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/victor-fdez/kube-route53-traefik/watch"
)

var dryRun bool

func main() {
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	flag.BoolVar(&dryRun, "dry-run", false, "do not update route53 when setting this flag")
	flag.Parse()
	glog.Info("Starting kubernetes / route53 / traefik synchronization service")
	if dryRun {
		glog.Info("Running in DRYRUN mode")
	}
	glog.Flush()
	watch.Setup(kubeconfig, dryRun)
	watch.Start()
}

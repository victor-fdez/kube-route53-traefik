package main

import (
	"flag"

	"github.com/golang/glog"
	watchKube "github.com/victor-fdez/kube-route53-traefik/watch"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	dryRun := flag.Bool("dry-run", false, "do not update route53 when setting this flag")
	flag.Parse()
	glog.Info("Starting kubernetes / route53 / traefik synchronization service")
	if *dryRun {
		glog.Info("running in DRYRUN mode")
	}
	glog.Flush()
	watchKube.Setup(kubeconfig, *dryRun)
	watchKube.Start()
}

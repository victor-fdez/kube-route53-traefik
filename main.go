package main

import (
	"flag"

	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	watchKube "github.com/victor-fdez/kube-route53-traefik/watch"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	flag.Parse()
	watchKube.Setup(kubeconfig)
	watchKube.Start()
}

package main

import (
	"flag"

	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dns_provider "github.com/victor-fdez/kube-route53-traefik/dns_providers"
	watchKube "github.com/victor-fdez/kube-route53-traefik/watch"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	flag.Parse()
	dns_provider.Setup()
	watchKube.Setup(kubeconfig)
	watchKube.Start()
}

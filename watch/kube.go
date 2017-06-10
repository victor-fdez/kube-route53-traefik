package watch

import (
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/victor-fdez/kube-route53-traefik/dns_providers"
	"github.com/victor-fdez/kube-route53-traefik/view"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	v1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var client *kubernetes.Clientset
var ingressWatcher, nodeWatcher watch.Interface
var serviceWatcherDone, nodeWatcherDone bool
var dryRun bool

func Setup(kubeconfig *string, DryRun bool) {
	var config *rest.Config
	dryRun = DryRun
	var err error
	//var serviceWatcherDone, nodeWatcherDone bool
	if *kubeconfig != "" {
		// uses the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}
	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	// setup watchers
	ingressWatcher, err = client.Ingresses("").Watch(v1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	nodeWatcher, err = client.Nodes().Watch(v1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	dns_providers.Setup(true)
	// setup AWS dns provider
	serviceWatcherDone = false
	nodeWatcherDone = false
}

func Start() {
	// get watcher for services in kubernetes
	//TODO: have diff structure to check changes
	//TODO: update aws after ingress is added
	id, subdomain := "ingress", "hello.waittimes.io"
	dns_providers.AddRoute(&id, &subdomain, []string{"8.8.8.8"})
	ingressEventChan := ingressWatcher.ResultChan()
	nodeEventChan := nodeWatcher.ResultChan()
	for {
		select {
		case event, ok := <-ingressEventChan:
			if ok {
				// process each event received
				ingress := event.Object.(*v1beta1.Ingress)
				routeChanges := view.State.UpdateIngress(ingress, event.Type)
				updateRoutes(routeChanges)
				view.State.Dump()
			} else {
				// error with channel/or no more events
				fmt.Printf("Error: no more service events")
				serviceWatcherDone = true
			}
		case event, ok := <-nodeEventChan:
			if ok {
				node := event.Object.(*v1.Node)
				routeChanges := view.State.UpdateNode(node, event.Type)
				updateRoutes(routeChanges)
				view.State.Dump()
			} else {
				fmt.Printf("Error: no more node events")
				nodeWatcherDone = true
			}
		}
		// if any or all of the channels are finished then
		// exit process
		if nodeWatcherDone || serviceWatcherDone {
			os.Exit(0)
		}
	}
}

func updateRoutes(routeChanges view.RouteChanges) error {
	id := ""
	for _, route := range routeChanges.Deleted {
		glog.Infof("Deleting route for %s", route.Subdomain)
		dns_providers.RemoveRoute(&id, &route.Subdomain)
	}
	for _, route := range routeChanges.Changed {
		glog.Infof("Upserting route for %s with %v", route.Subdomain, route.Ips)
		dns_providers.AddRoute(&id, &route.Subdomain, route.Ips)
	}
	if len(routeChanges.Deleted) == 0 && len(routeChanges.Changed) == 0 {
		glog.Infof("No changes to routes")
	}
	return nil
}

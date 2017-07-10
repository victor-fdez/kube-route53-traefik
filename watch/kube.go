package watch

import (
	"fmt"
	"os"

	"go.uber.org/zap"

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
var ingressWatcher, serviceWatcher, nodeWatcher watch.Interface
var ingressWatcherDone, serviceWatcherDone, nodeWatcherDone bool
var dryRun bool
var sLog *zap.SugaredLogger

func Setup(kubeconfig *string, DryRun bool, SLog *zap.SugaredLogger) {
	var err error
	var config *rest.Config
	dryRun = DryRun
	sLog = SLog
	//var serviceWatcherDone, nodeWatcherDone bool
	if *kubeconfig != "" {
		// uses the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			sLog.Panic(err)
		}
	} else {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			sLog.Panic(err)
		}
	}
	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		sLog.Panic(err)
	}
	// setup watchers
	ingressWatcher, err = client.Ingresses("").Watch(v1.ListOptions{})
	if err != nil {
		sLog.Panic(err)
	}
	// this service must be located in kube-system as the traefik ha proxy
	// will work as an ingress controller and it needs to be in this privileged
	// namespace
	serviceWatcher, err = client.Services("kube-system").Watch(v1.ListOptions{})
	if err != nil {
		sLog.Panic(err)
	}
	nodeWatcher, err = client.Nodes().Watch(v1.ListOptions{})
	if err != nil {
		sLog.Panic(err)
	}
	// setup the DNS provider, and cluster view
	dns_providers.Setup(dryRun, sLog)
	view.Setup(sLog)
	// setup AWS dns provider
	ingressWatcherDone = false
	serviceWatcherDone = false
	nodeWatcherDone = false
}

//TODO: find the ELB route from service load balancer specified, add an anotation to service
//TODO: specify either nodeport or service name to use
//TODO: add service watcher also for this kind of event

func Start() {
	// get watcher for services in kubernetes
	ingressEventChan := ingressWatcher.ResultChan()
	serviceEventChan := serviceWatcher.ResultChan()
	nodeEventChan := nodeWatcher.ResultChan()
	for {
		select {
		case event, ok := <-ingressEventChan:
			if ok {
				// process each event received
				ingress := event.Object.(*v1beta1.Ingress)
				sLog.Infof("%s ingress %s/%s with ingress controller [%v]",
					event.Type,
					ingress.Namespace,
					ingress.Name,
					ingress.Annotations["kubernetes.io/ingress.class"])
				routeChanges := view.State.UpdateIngress(ingress, event.Type)
				updateRoutes(routeChanges)
				view.State.Dump()
			} else {
				// error with channel/or no more events
				fmt.Printf("Error: no more service events")
				ingressWatcherDone = true
			}
		case event, ok := <-serviceEventChan:
			if ok {
				// process each event received
				service := event.Object.(*v1.Service)
				sLog.Infof("%s service %s with ingresses %v", event.Type, service.Name, service.Status.LoadBalancer.Ingress)
				routeChanges := view.State.UpdateIngCtrlSvc(service, event.Type)
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
				sLog.Infof("%s node %s with IP [%v]", event.Type, node.Name, node.Status.Addresses)
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
		if nodeWatcherDone ||
			serviceWatcherDone ||
			ingressWatcherDone {
			os.Exit(0)
		}
	}
}

func updateRoutes(routeChanges view.RouteChanges) error {
	id := ""
	for _, route := range routeChanges.Deleted {
		err := dns_providers.RemoveRoute(&id, &route.Subdomain, route.Alias)
		if err != nil {
			sLog.Warn(err)
		}
	}
	for _, route := range routeChanges.Changed {
		err := dns_providers.AddRoute(&id, &route.Subdomain, route.Ips, route.Alias)
		if err != nil {
			sLog.Warn(err)
		}
	}
	if len(routeChanges.Deleted) == 0 && len(routeChanges.Changed) == 0 {
		sLog.Infof("No changes to routes")
	}
	return nil
}

package main

import (
	"flag"
	"fmt"
	"os"

	watch "k8s.io/client-go/pkg/watch"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	var config *rest.Config
	var err error
	var serviceWatcherDone, nodeWatcherDone bool
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	flag.Parse()
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
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	// get watcher for services in kubernetes
	serviceWatcher, err := clientset.Services("").Watch(v1.ListOptions{ /*LabelSelector: "route53=true"*/ })
	if err != nil {
		panic(err.Error())
	}
	nodeWatcher, err := clientset.Nodes().Watch(v1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	serviceWatcherDone = false
	nodeWatcherDone = false
	serviceEventChan := serviceWatcher.ResultChan()
	nodeEventChan := nodeWatcher.ResultChan()
	for {
		select {
		case event, ok := <-serviceEventChan:
			if ok {
				// process each event received
				service := event.Object.(*v1.Service)
				fmt.Printf("For service: %#v with labels %#v\n", service.Name, service.Labels)
				switch event.Type {
				case watch.Added:
					fmt.Println("added")
				case watch.Modified:
					fmt.Println("modified")
				case watch.Deleted:
					fmt.Println("deleted")
				case watch.Error:
					fmt.Println("error")
				}
			} else {
				// error with channel/or no more events
				fmt.Printf("Error: no more service events")
				serviceWatcherDone = true
			}
		case event, ok := <-nodeEventChan:
			if ok {
				node := event.Object.(*v1.Node)
				machineID := node.Status.NodeInfo.MachineID
				addresses := node.Status.Addresses
				switch event.Type {
				case watch.Added:
					fmt.Printf("Node %v changed %v\n", machineID, addresses)
					updateNodeAddress(machineID, addresses["ExternalIP"])
				case watch.Modified:
					fmt.Printf("Node %v changed %v\n", machineID, addresses)
					updateNodeAddress(machineID, addresses["ExternalIP"])
				case watch.Deleted:
					fmt.Printf("Node %v deleted\n", machineID)
					deleteNodeAddress(machineID)
				}
				updateRoute53()
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

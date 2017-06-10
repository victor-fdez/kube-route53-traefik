package view

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	messagediff "gopkg.in/d4l3k/messagediff.v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/watch"
)

type Ingress struct {
	name      string
	namespace string
	hostnames []string
}

type Node struct {
	MID        string
	externalIP string
}

type LBTraefik struct {
	aliasDNS string
}

type ClusterView struct {
	ingresses map[string]Ingress
	nodes     map[string]Node
	lbTraefik LBTraefik
}

type RouteChanges struct {
	Deleted []Route
	Changed []Route
}

type Route struct {
	Subdomain string
	Ips       []string
}

var State ClusterView

func init() {
	State = ClusterView{
		ingresses: make(map[string]Ingress),
		nodes:     make(map[string]Node),
	}
}

func ingressKey(i *v1beta1.Ingress) string {
	return i.ObjectMeta.Namespace + "/" + i.ObjectMeta.Name
}

func createIngress(i *v1beta1.Ingress) Ingress {
	// add all of the hosts for the ingress
	hosts := make([]string, 0, len(i.Spec.Rules))
	for _, rule := range i.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	//TODO: order host names
	newIngress := Ingress{
		name:      i.ObjectMeta.Name,
		namespace: i.ObjectMeta.Namespace,
		hostnames: hosts,
	}
	return newIngress
}

func (c ClusterView) Dump() {
	spew.Config.Indent = "\t"
	spew.Dump(c)
}

func (c ClusterView) UpdateIngress(ingress *v1beta1.Ingress, eventType watch.EventType) RouteChanges {
	routeChanges := RouteChanges{
		Deleted: make([]Route, 0, 1),
		Changed: make([]Route, 0, 1),
	}
	//TODO: if ingress is added then add/modified route is returned
	switch eventType {
	case watch.Added:
		routeChanges.Changed = State.AddIngress(ingress)
	case watch.Modified:
		routeChanges.Changed = State.ModIngress(ingress)
	case watch.Deleted:
		routeChanges.Deleted = State.DeleteIngress(ingress)
	case watch.Error:
		fmt.Println("error")
	}
	return routeChanges
}

func (c ClusterView) UpdateNode(node *v1.Node, eventType watch.EventType) RouteChanges {
	routeChanges := RouteChanges{
		Deleted: make([]Route, 0, 1),
		Changed: make([]Route, 0, 1),
	}
	switch eventType {
	case watch.Added:
		routeChanges.Changed = State.AddNode(node)
	case watch.Modified:
		routeChanges.Changed = State.ModNode(node)
	case watch.Deleted:
		routeChanges.Deleted = State.DeleteNode(node)
	}
	return routeChanges
}

func (c ClusterView) AddIngress(i *v1beta1.Ingress) []Route {
	key := ingressKey(i)
	_, ok := c.ingresses[key]
	if ok {
		panic(fmt.Sprintf("Ingress already added - %#v\n", i))
	}
	newIngress := createIngress(i)
	c.ingresses[key] = newIngress
	routes := c.createRoutes(newIngress.hostnames)
	return routes
}

func (c ClusterView) DeleteIngress(i *v1beta1.Ingress) []Route {
	key := ingressKey(i)
	_, ok := c.ingresses[key]
	if ok {
		delete(c.ingresses, key)
		fmt.Printf("Deleted Ingress with key = %v\n", key)
	}
	oldIngress := createIngress(i)
	routes := c.createRoutes(oldIngress.hostnames)
	return routes
}

func (c ClusterView) ModIngress(i *v1beta1.Ingress) []Route {
	key := ingressKey(i)
	ingress, ok := c.ingresses[key]
	if !ok {
		panic(fmt.Sprintf("Ingress does not exists but was modifed %#v", i))
	}
	newIngress := createIngress(i)
	_, equal := messagediff.DeepDiff(ingress, newIngress)
	if equal {
		return make([]Route, 0)
	}
	c.ingresses[key] = newIngress
	routes := c.createRoutes(newIngress.hostnames)
	return routes
}

func nodeKey(node *v1.Node) string {
	return node.Status.NodeInfo.MachineID
}

func createNode(node *v1.Node) Node {
	var ip string
	for _, address := range node.Status.Addresses {
		if address.Type == v1.NodeExternalIP {
			ip = address.Address
		}
	}
	return Node{
		MID:        node.Status.NodeInfo.MachineID,
		externalIP: ip,
	}
}

func (c ClusterView) AddNode(node *v1.Node) []Route {
	key := nodeKey(node)
	_, ok := c.nodes[key]
	if ok {
		panic(fmt.Sprintf("Ingress already added - %#v\n", node))
	}
	newNode := createNode(node)
	c.nodes[key] = newNode
	hostnames := c.getHostnames()
	routes := c.createRoutes(hostnames)
	return routes
}

func (c ClusterView) DeleteNode(node *v1.Node) []Route {
	key := nodeKey(node)
	_, ok := c.nodes[key]
	if ok {
		delete(c.nodes, key)
		fmt.Printf("Deleted node with key = %v\n", key)
	}
	hostnames := c.getHostnames()
	routes := c.createRoutes(hostnames)
	return routes
}

func (c ClusterView) ModNode(node *v1.Node) []Route {
	key := nodeKey(node)
	oldNode, ok := c.nodes[key]
	if !ok {
		panic(fmt.Sprintf("Node does not exists but was modifed %#v", node))
	}
	newNode := createNode(node)
	_, equal := messagediff.DeepDiff(oldNode, newNode)
	if equal {
		return make([]Route, 0)
	}
	c.nodes[key] = newNode
	hostnames := c.getHostnames()
	routes := c.createRoutes(hostnames)
	return routes
}

func (c ClusterView) getNodeIps() []string {
	ips := make([]string, 0, 3)
	for _, node := range c.nodes {
		ips = append(ips, node.externalIP)
	}
	return ips
}

func (c ClusterView) getHostnames() []string {
	hostnames := make([]string, 0, 3)
	for _, ingress := range c.ingresses {
		ingressHostnames := ingress.hostnames
		hostnames = append(hostnames, ingressHostnames...)
	}
	return hostnames
}

func (c ClusterView) createRoutes(hostnames []string) []Route {
	routes := make([]Route, 0, 1)
	ips := c.getNodeIps()
	if len(hostnames) != 0 && len(ips) != 0 {
		for _, hostname := range hostnames {
			routes = append(routes, Route{
				Subdomain: hostname,
				Ips:       ips,
			})
		}
	}
	return routes
}

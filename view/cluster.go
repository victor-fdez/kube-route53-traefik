package view

import (
	"fmt"

	"go.uber.org/zap"

	messagediff "gopkg.in/d4l3k/messagediff.v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/watch"
)

type Ingress struct {
	name        string
	namespace   string
	hostnames   []string
	ingCtrlName string
}

type Node struct {
	mID        string
	externalIP string
}

// IngressCtrl contains the name of the ingress controller
// and the CN name (LBAlias) pointing to the load balancer
// to the ingress controller.
type IngressCtrl struct {
	Name    string
	SvcName string
	LBAlias string
}

func (i *IngressCtrl) init(svc *v1.Service) {
	i.Name = svc.Name
	i.Name = svc.Annotations["route-ing-ctrl"]
	ings := svc.Status.LoadBalancer.Ingress
	if len(ings) == 1 {
		i.LBAlias = ings[0].Hostname
	} else {
		// TODO: support more ingresses for different cloud providers
		sLog.Warn("Currently we only support ELB AWS routes")
	}
}

// ClusterView contains current view of the kubernetes
// cluster if any information pertaining to either
// nodes in the cluster, ingresses, or ingress controllers
// (their services) then this struct will contain up-to-date
// information
type ClusterView struct {
	// ingresses where traffic will be redirected by
	// ingress controllers
	ings  map[string]Ingress
	nodes map[string]Node
	// ingress controllers monitor ingresses and redirect
	// traffic if they are specified to redirect traffic
	// using kubernetes.io/ingress.class: ingCtrls(name)
	ingCtrls map[string]IngressCtrl
}

type RouteChanges struct {
	Deleted []Route
	Changed []Route
}

type Route struct {
	Subdomain string
	Ips       []string
	Alias     string
	UseAlias  bool
}

func NoRoutes() RouteChanges {
	return RouteChanges{
		Deleted: []Route{},
		Changed: []Route{},
	}
}

var State ClusterView
var sLog *zap.SugaredLogger

func Setup(SLog *zap.SugaredLogger) {
	State = ClusterView{
		ings:     make(map[string]Ingress),
		nodes:    make(map[string]Node),
		ingCtrls: make(map[string]IngressCtrl),
	}
	sLog = SLog
}

func ingressKey(i *v1beta1.Ingress) string {
	return i.ObjectMeta.Namespace + "/" + i.ObjectMeta.Name
}

func (c ClusterView) ingressAlias(i Ingress) *string {
	ingCtrl, ok := c.ingCtrls[i.ingCtrlName]
	if i.ingCtrlName != "" && ok {
		return &ingCtrl.LBAlias
	}
	return nil
}

func key(s *v1.Service) (string, bool) {
	if val, ok := s.Annotations["route-ing-ctrl"]; ok {
		return val, true
	}
	return "", false
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
	// get ingress controller name
	if val, ok := i.Annotations["kubernetes.io/ingress.class"]; ok {
		newIngress.ingCtrlName = val
	}
	return newIngress
}

func (c ClusterView) Dump() {
	//sLog.Debug(spew.Sdump(c))
}

func (c ClusterView) UpdateIngress(ingress *v1beta1.Ingress, eventType watch.EventType) RouteChanges {
	var routeChanges RouteChanges
	switch eventType {
	case watch.Added:
		routeChanges = State.AddIngress(ingress)
	case watch.Modified:
		routeChanges = State.ModIngress(ingress)
	case watch.Deleted:
		routeChanges = State.DeleteIngress(ingress)
	case watch.Error:
		fmt.Println("error")
	}
	return routeChanges
}

func (c ClusterView) UpdateNode(node *v1.Node, eventType watch.EventType) RouteChanges {
	var routeChanges RouteChanges

	switch eventType {
	case watch.Added:
		routeChanges = State.AddNode(node)
	case watch.Modified:
		routeChanges = State.ModNode(node)
	case watch.Deleted:
		routeChanges = State.DeleteNode(node)
	}
	return routeChanges
}

func (c ClusterView) UpdateIngCtrlSvc(svc *v1.Service, eventType watch.EventType) RouteChanges {
	var routeChanges RouteChanges
	switch eventType {
	case watch.Added:
		routeChanges = State.addCtrlSvc(svc)
	case watch.Deleted:
		routeChanges = State.delCtrlSvc(svc)
	}
	return routeChanges
}

func (c ClusterView) AddIngress(i *v1beta1.Ingress) RouteChanges {
	key := ingressKey(i)
	_, ok := c.ings[key]
	if ok {
		sLog.Panic(fmt.Sprintf("Ingress already added - %#v\n", i))
	}
	newIngress := createIngress(i)
	c.ings[key] = newIngress
	alias := c.ingressAlias(newIngress)
	return RouteChanges{
		Deleted: []Route{},
		Changed: c.createRoutes(newIngress.hostnames, alias),
	}
}

func (c ClusterView) DeleteIngress(i *v1beta1.Ingress) RouteChanges {
	key := ingressKey(i)
	_, ok := c.ings[key]
	if ok {
		delete(c.ings, key)
		sLog.Infof("Deleted Ingress with key = %v\n", key)
	}
	oldIngress := createIngress(i)
	changes := RouteChanges{
		Deleted: []Route{},
		Changed: []Route{},
	}
	if len(c.nodes) != 0 {
		alias := c.ingressAlias(oldIngress)
		changes.Deleted = c.createRoutes(oldIngress.hostnames, alias)
	}
	return changes
}

func (c ClusterView) ModIngress(i *v1beta1.Ingress) RouteChanges {
	key := ingressKey(i)
	ingress, ok := c.ings[key]
	if !ok {
		sLog.Panic(fmt.Sprintf("Ingress does not exists but was modifed %#v", i))
	}
	newIngress := createIngress(i)
	_, equal := messagediff.DeepDiff(ingress, newIngress)
	if equal {
		return RouteChanges{
			Deleted: []Route{},
			Changed: []Route{},
		}
	}
	c.ings[key] = newIngress
	alias := c.ingressAlias(newIngress)
	return RouteChanges{
		Deleted: c.createRoutes(ingress.hostnames, alias),
		Changed: c.createRoutes(newIngress.hostnames, alias),
	}
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
		mID:        node.Status.NodeInfo.MachineID,
		externalIP: ip,
	}
}

func (c ClusterView) AddNode(node *v1.Node) RouteChanges {
	key := nodeKey(node)
	_, ok := c.nodes[key]
	if ok {
		sLog.Panic(fmt.Sprintf("Node already added - %#v\n", node))
	}
	newNode := createNode(node)
	c.nodes[key] = newNode
	hostnames := c.getHostnames(false, "")
	return RouteChanges{
		Deleted: []Route{},
		Changed: c.createRoutes(hostnames, nil),
	}
}

func (c ClusterView) DeleteNode(node *v1.Node) RouteChanges {
	key := nodeKey(node)
	_, ok := c.nodes[key]
	if ok {
		delete(c.nodes, key)
		sLog.Infof("Deleted node with key = %v\n", key)
	}
	hostnames := c.getHostnames(false, "")
	return RouteChanges{
		Deleted: []Route{},
		Changed: c.createRoutes(hostnames, nil),
	}
}

func (c ClusterView) ModNode(node *v1.Node) RouteChanges {
	key := nodeKey(node)
	oldNode, ok := c.nodes[key]
	if !ok {
		sLog.Panic(fmt.Sprintf("Node does not exists but was modifed %#v", node))
	}
	newNode := createNode(node)
	_, equal := messagediff.DeepDiff(oldNode, newNode)
	if equal {
		return RouteChanges{
			Deleted: []Route{},
			Changed: []Route{},
		}
	}
	c.nodes[key] = newNode
	hostnames := c.getHostnames(false, "")
	return RouteChanges{
		Deleted: []Route{},
		Changed: c.createRoutes(hostnames, nil),
	}
}

func (c ClusterView) addCtrlSvc(svc *v1.Service) RouteChanges {
	var ingCtrl IngressCtrl
	key, ok := key(svc)
	if !ok {
		sLog.Infof("Ingress does not have annotation to be used by routing")
		return NoRoutes()
	}
	// we already have this service
	_, ok = c.ingCtrls[key]
	if ok {
		sLog.Panic(fmt.Sprintf("Service already exists %#v", svc))
	}
	// add service and generate new routes if ingresses depend on this
	// ingress controller
	ingCtrl.init(svc)
	c.ingCtrls[key] = ingCtrl
	hostnames := c.getHostnames(true, ingCtrl.Name)
	sLog.Infof("Got aliasable hostnames [%v]", hostnames)
	return RouteChanges{
		Deleted: []Route{},
		Changed: c.createRoutes(hostnames, &ingCtrl.LBAlias),
	}
}

func (c ClusterView) delCtrlSvc(svc *v1.Service) RouteChanges {
	var ing IngressCtrl
	key, ok := key(svc)
	if !ok {
		sLog.Infof("Ingress does not have annotation to be used by routing")
		return NoRoutes()
	}
	// if we don't have the service how can it be deleted
	ing, ok = c.ingCtrls[key]
	if !ok {
		sLog.Panic(fmt.Sprintf("Service didn't exists but is being deleted %#v", svc))
	}
	delete(c.ingCtrls, key)
	// add service and generate new routes if ingresses depend on this
	// ingress controller
	hostnames := c.getHostnames(true, ing.Name)
	sLog.Infof("Got aliasable hostnames [%v]", hostnames)
	return RouteChanges{
		Deleted: c.createRoutes(hostnames, &ing.LBAlias),
		Changed: []Route{},
	}
}

func (c ClusterView) getNodeIps() []string {
	ips := make([]string, 0, 3)
	for _, node := range c.nodes {
		ips = append(ips, node.externalIP)
	}
	return ips
}

func (c ClusterView) getHostnames(onlyAliasable bool, ingCtrlName string) []string {
	hostnames := make([]string, 0, 3)
	for _, ingress := range c.ings {
		if onlyAliasable && ingress.ingCtrlName == ingCtrlName {
			ingressHostnames := ingress.hostnames
			hostnames = append(hostnames, ingressHostnames...)
		} else if !onlyAliasable && ingress.ingCtrlName == "" {
			// only get this hostnames if aren't setup for ingress controllers
			ingressHostnames := ingress.hostnames
			hostnames = append(hostnames, ingressHostnames...)
		}
	}
	return hostnames
}

// createRoutes will create AA routes with ips whenever ingCtrls is nil, else
// it will create AA alias routes
func (c ClusterView) createRoutes(hostnames []string, alias *string) []Route {
	var ips []string
	ipRoutes := make([]Route, 0, 1)
	// If we don't have an alias then use the IPs of the nodes
	if alias == nil {
		ips = c.getNodeIps()
	}
	if len(hostnames) != 0 &&
		(alias != nil && len(ips) == 0) ||
		(alias == nil && len(ips) != 0) {
		for _, hostname := range hostnames {
			if alias == nil {
				ipRoutes = append(ipRoutes, Route{
					Subdomain: hostname,
					Ips:       ips,
					UseAlias:  false,
					Alias:     "",
				})
			} else {
				ipRoutes = append(ipRoutes, Route{
					Subdomain: hostname,
					Ips:       []string{},
					UseAlias:  true,
					Alias:     *alias,
				})
			}
		}
	}
	return ipRoutes
}

func (c ClusterView) getIngCtrlHostnames(ingCtrlName string) []string {
	hostnames := make([]string, 0, 1)
	for _, ingress := range c.ings {
		if ingress.ingCtrlName == ingCtrlName {
			ingressHostnames := ingress.hostnames
			hostnames = append(hostnames, ingressHostnames...)
		}
	}
	return hostnames
}

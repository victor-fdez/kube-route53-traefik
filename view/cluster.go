package view

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	messagediff "gopkg.in/d4l3k/messagediff.v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
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
func (c ClusterView) AddIngress(i *v1beta1.Ingress) bool {
	key := ingressKey(i)
	_, ok := c.ingresses[key]
	if ok {
		panic(fmt.Sprintf("Ingress already added - %#v\n", i))
	}
	newIngress := createIngress(i)
	c.ingresses[key] = newIngress
	return true
}

func (c ClusterView) DeleteIngress(i *v1beta1.Ingress) bool {
	key := ingressKey(i)
	_, ok := c.ingresses[key]
	if ok {
		delete(c.ingresses, key)
		fmt.Printf("Deleted Ingress with key = %v\n", key)
	}
	return true
}

func (c ClusterView) ModIngress(i *v1beta1.Ingress) bool {
	key := ingressKey(i)
	ingress, ok := c.ingresses[key]
	if !ok {
		panic(fmt.Sprintf("Ingress does not exists but was modifed %#v", i))
	}
	newIngress := createIngress(i)
	_, equal := messagediff.DeepDiff(ingress, newIngress)
	if equal {
		return false
	}
	c.ingresses[key] = newIngress
	return true
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

func (c ClusterView) AddNode(node *v1.Node) bool {
	key := nodeKey(node)
	_, ok := c.nodes[key]
	if ok {
		panic(fmt.Sprintf("Ingress already added - %#v\n", node))
	}
	newNode := createNode(node)
	c.nodes[key] = newNode
	return true
}

func (c ClusterView) DeleteNode(node *v1.Node) bool {
	key := nodeKey(node)
	_, ok := c.nodes[key]
	if ok {
		delete(c.nodes, key)
		fmt.Printf("Deleted node with key = %v\n", key)
	}
	return true
}

func (c ClusterView) ModNode(node *v1.Node) bool {
	key := nodeKey(node)
	oldNode, ok := c.nodes[key]
	if !ok {
		panic(fmt.Sprintf("Node does not exists but was modifed %#v", node))
	}
	newNode := createNode(node)
	_, equal := messagediff.DeepDiff(oldNode, newNode)
	if equal {
		return false
	}
	c.nodes[key] = newNode
	return true
}

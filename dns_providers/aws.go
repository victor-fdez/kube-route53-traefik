package dns_provider

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

var route53Svc *route53.Route53

type Routes struct {
}

func Setup(credentialFile *string) {
	session, err := session.Must(session.NewSession())
	if err != nil {
		panic(err.Error())
	}
	route53Svc = route53.New(session)
}

func GetRoutes() error {
	route, err := getDestinationZone("rabbit.waittimes.io", route53Svc)
	if err != nil {
		return fmt.Errorf("Unable to get waittimes.io")
	}
	return nil
}

func getDestinationZone(domain string, r53Api *route53.Route53) (*route53.HostedZone, error) {
	tld, err := getTLD(domain)
	if err != nil {
		return nil, err
	}

	listHostedZoneInput := route53.ListHostedZonesByNameInput{
		DNSName: &tld,
	}
	hzOut, err := r53Api.ListHostedZonesByName(&listHostedZoneInput)
	if err != nil {
		return nil, fmt.Errorf("No zone found for %s: %v", tld, err)
	}
	// TODO: The AWS API may return multiple pages, we should parse them all
	return findMostSpecificZoneForDomain(domain, hzOut.HostedZones)
}

func getTLD(domain string) (string, error) {
	domainParts := strings.Split(domain, ".")
	segments := len(domainParts)
	if segments < 3 {
		return "", fmt.Errorf(
			"Domain %s is invalid - it should be a fully qualified domain name and subdomain (i.e. test.example.com)",
			domain)
	}
	return strings.Join(domainParts[segments-2:], "."), nil
}

func findMostSpecificZoneForDomain(domain string, zones []*route53.HostedZone) (*route53.HostedZone, error) {
	domain = domainWithTrailingDot(domain)
	if len(zones) < 1 {
		return nil, fmt.Errorf("No zone found for %s", domain)
	}
	var mostSpecific *route53.HostedZone
	curLen := 0

	for i := range zones {
		fmt.Printf("domain: %v checking %v\n", domain, zoneName)
		zone := zones[i]
		zoneName := *zone.Name

		if (domain == zoneName || strings.HasSuffix(domain, "."+zoneName)) && curLen < len(zoneName) {
			curLen = len(zoneName)
			mostSpecific = zone
		}
	}

	if mostSpecific == nil {
		return nil, fmt.Errorf("Zone found %s does not match domain given %s", *zones[0].Name, domain)
	}

	return mostSpecific, nil
}

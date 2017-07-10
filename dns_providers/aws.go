package dns_providers

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	messagediff "gopkg.in/d4l3k/messagediff.v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

var route53Svc *route53.Route53
var dryRun bool
var routes AWSRoutes
var sLog *zap.SugaredLogger

type Route struct {
	subdomain  string
	domain     string
	ips        []string
	alias      string
	hostedZone *route53.HostedZone
}
type AWSRoutes map[string]Route

//TODO: add support for alias routes
func Setup(DryRun bool, SLog *zap.SugaredLogger) {
	routes = make(AWSRoutes)
	session := session.Must(session.NewSession())
	route53Svc = route53.New(session)
	dryRun = DryRun
	sLog = SLog
	sLog.Infof("Running in DRYRUN mode")
}

func AddRoute(id, subdomain *string, ips []string, alias string) error {
	var subdomainRoute Route
	var ok bool = false
	key := *id + "/" + *subdomain

	subdomainRoute, ok = routes[key]
	if !ok {
		tld, route, err := getDestinationZone(*subdomain, route53Svc)
		if err != nil {
			return fmt.Errorf("Unable to get hosted zone for %s", *subdomain)
		}
		subdomainRoute = Route{
			subdomain:  *subdomain,
			domain:     *tld,
			hostedZone: route,
			alias:      alias,
			ips:        ips,
		}
		sLog.Infof("adding subdomain (%s) to domain (%s)", subdomainRoute.subdomain, subdomainRoute.domain)
	} else {
		tld, route, err := getDestinationZone(*subdomain, route53Svc)
		if err != nil {
			return err
		}
		subdomainRouteNew := Route{
			subdomain:  *subdomain,
			domain:     *tld,
			hostedZone: route,
			alias:      alias,
			ips:        ips,
		}
		sLog.Infof("Found route in stored routes checking if something has changed (%s)", key)
		// check if something changed for structure
		diff, equal := messagediff.DeepDiff(subdomainRouteNew, subdomainRoute)
		if equal {
			// do nothing we already have routes setup
			return nil
		}
		sLog.Infof("Routes differed %v", diff)
		// transplant previous information to new structure
		subdomainRoute = subdomainRouteNew
	}

	//TODO: for now just with multiple IPs in the future may use alias
	err := updateDNS(route53Svc,
		subdomainRoute.ips,
		subdomainRoute.alias,
		subdomainRoute.subdomain,
		*subdomainRoute.hostedZone.Id)
	if err != nil {
		return fmt.Errorf("Unable to update route53 for subdomain %s : %v", subdomainRoute.subdomain, err)
	} else {
		routes[key] = subdomainRoute
	}
	return nil
}

func RemoveRoute(id, subdomain *string, alias string) error {
	key := *id + "/" + *subdomain

	subdomainRoute, ok := routes[key]
	if !ok {
		// There's nothing to delete hmmm
		return fmt.Errorf("Unable to delete any AWS routes since the route does not exists (%s)", key)
	}
	err := removeDNS(route53Svc,
		subdomainRoute.ips,
		alias,
		subdomainRoute.subdomain,
		*subdomainRoute.hostedZone.Id)
	if err != nil {
		return fmt.Errorf("Unable to delete route53 for subdomain %s", subdomainRoute.subdomain)
	} else {
		// delete route from routes if successfully deleted from route53
		delete(routes, key)
	}
	return nil
}

func updateDNS(r53Api *route53.Route53, ips []string, alias string, domain, zoneID string) error {
	var resourceRecords []*route53.ResourceRecord = make([]*route53.ResourceRecord, 0, 1)
	var rrs route53.ResourceRecordSet
	var cleanDomain = strings.Trim(domain, ".") + "."
	var TTL int64 = 300
	var EzoneID = "Z35SXDOTRQ7X7K"
	zoneID = strings.Split(zoneID, "/")[2]
	// If we have an alias we use that
	if alias != "" {
		at := route53.AliasTarget{
			DNSName:              &alias,
			EvaluateTargetHealth: aws.Bool(false),
			HostedZoneId:         &EzoneID,
		}
		rrs = route53.ResourceRecordSet{
			AliasTarget: &at,
			Name:        &cleanDomain,
			Type:        aws.String("A"),
		}
		sLog.Infof("UPSERT A Record in zone %s for domain %s with Alias [%s]", zoneID, domain, alias)
	} else {
		// for multiple ips we use those ips instead
		for i, _ := range ips {
			rr := route53.ResourceRecord{
				Value: &ips[i],
			}
			resourceRecords = append(resourceRecords, &rr)
		}
		// A record for multiple IPs
		rrs = route53.ResourceRecordSet{
			ResourceRecords: resourceRecords,
			Name:            &cleanDomain,
			Type:            aws.String("A"),
			TTL:             &TTL,
		}
		sLog.Infof("UPSERT A Record in zone %s for domain %s with IP addresses %v", zoneID, domain, ips)
	}
	change := route53.Change{
		Action:            aws.String("UPSERT"),
		ResourceRecordSet: &rrs,
	}
	batch := route53.ChangeBatch{
		Changes: []*route53.Change{&change},
		Comment: aws.String("Kubernetes Update to Service"),
	}
	crrsInput := route53.ChangeResourceRecordSetsInput{
		ChangeBatch:  &batch,
		HostedZoneId: &zoneID,
	}
	if dryRun {
		sLog.Infof("DRY RUN: We normally would have updated %s (%s) to point to %#v", domain, zoneID, rrs)
		return nil
	}
	_, err := r53Api.ChangeResourceRecordSets(&crrsInput)
	if err != nil {
		return fmt.Errorf("Failed to update record set: %v", err.Error())
	}
	return nil
}

func removeDNS(r53Api *route53.Route53, ips []string, alias string, domain, zoneID string) error {
	var resourceRecords []*route53.ResourceRecord = make([]*route53.ResourceRecord, 0, 1)
	var rrs route53.ResourceRecordSet
	var TTL int64 = 300
	var EzoneID = "Z35SXDOTRQ7X7K"
	zoneID = strings.Split(zoneID, "/")[2]
	// If we have an alias we use that
	if alias != "" {
		at := route53.AliasTarget{
			DNSName:              &alias,
			EvaluateTargetHealth: aws.Bool(false),
			HostedZoneId:         &EzoneID,
		}
		rrs = route53.ResourceRecordSet{
			AliasTarget: &at,
			Name:        &domain,
			Type:        aws.String("A"),
		}
		sLog.Infof("DELETE A Record in zone %s for domain %s with Alias [%s]", zoneID, domain, alias)
	} else {
		// for multiple ips we use those ips instead
		for i, _ := range ips {
			rr := route53.ResourceRecord{
				Value: &ips[i],
			}
			resourceRecords = append(resourceRecords, &rr)
		}
		// A record for multiple IPs
		rrs = route53.ResourceRecordSet{
			ResourceRecords: resourceRecords,
			Name:            &domain,
			Type:            aws.String("A"),
			TTL:             &TTL,
		}
		sLog.Infof("DELETE A Record in zone %s for domain %s with IP addresses %v", zoneID, domain, ips)
	}
	change := route53.Change{
		Action:            aws.String("DELETE"),
		ResourceRecordSet: &rrs,
	}
	batch := route53.ChangeBatch{
		Changes: []*route53.Change{&change},
		Comment: aws.String("Kubernetes Update to Service"),
	}
	crrsInput := route53.ChangeResourceRecordSetsInput{
		ChangeBatch:  &batch,
		HostedZoneId: &zoneID,
	}
	if dryRun {
		sLog.Infof("DRY RUN: We normally would have deleted %s (%s) pointing to %#v", domain, zoneID, rrs)
		return nil
	}
	_, err := r53Api.ChangeResourceRecordSets(&crrsInput)
	if err != nil {
		return fmt.Errorf("Failed to delete record set: %v", err)
	}
	return nil
}

func getDestinationZone(domain string, r53Api *route53.Route53) (*string, *route53.HostedZone, error) {
	tld, err := getTLD(domain)
	if err != nil {
		return nil, nil, err
	}

	listHostedZoneInput := route53.ListHostedZonesByNameInput{
		DNSName: &tld,
	}
	hzOut, err := r53Api.ListHostedZonesByName(&listHostedZoneInput)
	if err != nil {
		return nil, nil, fmt.Errorf("No zone found for %s: %v", tld, err)
	}
	hz, err := findMostSpecificZoneForDomain(domain, hzOut.HostedZones)
	return &tld, hz, err
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
		zone := zones[i]
		zoneName := *zone.Name
		if dryRun {
			sLog.Infof("domain: %v checking %v", domain, zoneName)
		}
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

func domainWithTrailingDot(withoutDot string) string {
	if withoutDot[len(withoutDot)-1:] == "." {
		return withoutDot
	}
	return fmt.Sprint(withoutDot, ".")
}

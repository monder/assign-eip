package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var ec2Service *ec2.EC2
var metadataService *ec2metadata.EC2Metadata

func main() {
	flag.Parse()
	validAddresses := flag.Args()
	if len(validAddresses) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s <ip-or-cidr> [<ip-or-cider>...]\n", os.Args[0])
		os.Exit(1)
	}

	metadataService = ec2metadata.New(session.New())
	identity, err := metadataService.GetInstanceIdentityDocument()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to detect instance id and region: %s\n", err)
		os.Exit(1)
	}
	ec2Service = ec2.New(session.New(&aws.Config{Region: aws.String(identity.Region)}))

	allocatedIPs, err := getAllEIPs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to retrieve avalable elastic IPs: %s\n", err)
		os.Exit(1)
	}

	var freeEIP string
	for _, ip := range allocatedIPs {
		if ip.InstanceId != nil && *ip.InstanceId == identity.InstanceID {
			fmt.Println("IP is already assigned")
			select {} // Sleep forever
		}
		if freeEIP == "" && ip.InstanceId == nil {
			valid, err := isAddressValid(*ip.PublicIp, validAddresses)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to validate EIP: %s\n", err)
				os.Exit(1)
			}
			if valid {
				freeEIP = *ip.AllocationId
			}
		}
	}

	if freeEIP == "" {
		fmt.Fprintf(os.Stderr, "No usable IPs found. Checked %d entries.\n", len(allocatedIPs))
		os.Exit(1)
	}

	eni, err := getPrimaryENI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to detect primary network interface: %s\n", err)
		os.Exit(1)
	}
	_, err = ec2Service.AssociateAddress(&ec2.AssociateAddressInput{
		AllocationId:       aws.String(freeEIP),
		NetworkInterfaceId: aws.String(eni),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to associate address: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Assigned %s to %s.\n", freeEIP, eni)
	select {} // Sleep forever
}

func getAllEIPs() ([]*ec2.Address, error) {
	result, err := ec2Service.DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("domain"),
				Values: []*string{
					aws.String("vpc"),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return result.Addresses, nil
}

func isAddressValid(address string, validAddresses []string) (bool, error) {
	ip := net.ParseIP(address)
	if ip == nil {
		return false, fmt.Errorf("invalid ip: %s", address)
	}
	for _, validIP := range validAddresses {
		// Check if its CIDR
		_, cidr, err := net.ParseCIDR(validIP)
		if err == nil {
			if cidr.Contains(ip) {
				return true, nil
			}
		} else if net.ParseIP(validIP) == nil {
			// Its not IP nor CIDR
			return false, fmt.Errorf("invalid ip or cidr: %s", validIP)
		} else {
			if address == validIP {
				return true, nil
			}
		}
	}
	return false, nil
}

func getPrimaryENI() (string, error) {
	mac, err := metadataService.GetMetadata("mac")
	if err != nil {
		return "", err
	}
	eni, err := metadataService.GetMetadata("network/interfaces/macs/" + mac + "/interface-id")
	if err != nil {
		return "", err
	}
	return eni, nil
}

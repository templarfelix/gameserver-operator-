package main

import (
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		conf := config.New(ctx, "")

		region := conf.Get("region")
		clusterName := conf.Require("clusterName")

		// Criar a VPC
		vpc, err := compute.NewNetwork(ctx, "gameserver-operator-vpc", &compute.NetworkArgs{
			Name:                  pulumi.String("gameserver-operator-vpc"),
			AutoCreateSubnetworks: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Criar um Internet Gateway
		router, err := compute.NewRouter(ctx, "gameserver-operator-router", &compute.RouterArgs{
			Name:    pulumi.String("gameserver-operator-router"),
			Network: vpc.Name,
			Region:  pulumi.String(region),
		}, pulumi.DependsOn([]pulumi.Resource{vpc}))
		if err != nil {
			return err
		}

		// Criar um Cloud NAT para permitir acesso à internet para instâncias sem IPs públicos
		_, err = compute.NewRouterNat(ctx, "gameserver-operator-nat", &compute.RouterNatArgs{
			Name:                          pulumi.String("gameserver-operator-nat"),
			Router:                        pulumi.String("gameserver-operator-router"),
			Region:                        pulumi.String(region),
			NatIpAllocateOption:           pulumi.String("AUTO_ONLY"),
			SourceSubnetworkIpRangesToNat: pulumi.String("ALL_SUBNETWORKS_ALL_IP_RANGES"),
		}, pulumi.DependsOn([]pulumi.Resource{vpc, router}))
		if err != nil {
			return err
		}

		cluster, err := container.NewCluster(ctx, clusterName, &container.ClusterArgs{
			Name:             pulumi.String(clusterName),
			InitialNodeCount: pulumi.Int(1),
			Location:         pulumi.String(region),
			Network:          vpc.Name, // Atualizado para usar a nova VPC
			ReleaseChannel: &container.ClusterReleaseChannelArgs{
				Channel: pulumi.String("RAPID"),
			},
			DatapathProvider: pulumi.String("ADVANCED_DATAPATH"),
			DnsConfig: &container.ClusterDnsConfigArgs{
				ClusterDns:       pulumi.String("CLOUD_DNS"),
				ClusterDnsScope:  pulumi.String("VPC_SCOPE"),
				ClusterDnsDomain: pulumi.String("templarfelix.com"),
			},
			RemoveDefaultNodePool: pulumi.Bool(true),
			DeletionProtection:    pulumi.Bool(false),
		}, pulumi.DependsOn([]pulumi.Resource{vpc}))
		if err != nil {
			return err
		}

		_, err = container.NewNodePool(ctx, "gameserver-operator-node-pool", &container.NodePoolArgs{
			Name:      pulumi.String("gameserver-operator-node-pool"),
			Cluster:   cluster.ID(),
			NodeCount: pulumi.Int(1),
			Location:  pulumi.String(region),
			NodeConfig: &container.NodePoolNodeConfigArgs{
				Spot:        pulumi.Bool(true),
				MachineType: pulumi.String("n1-standard-8"),
				OauthScopes: pulumi.StringArray{
					pulumi.String("https://www.googleapis.com/auth/cloud-platform"),
				},
			},
		})
		if err != nil {
			return err
		}

		// Criar um IP público
		ip, err := compute.NewAddress(ctx, "game-ip", &compute.AddressArgs{
			Region: pulumi.String(region),
		})
		if err != nil {
			return err
		}

		// Configurar regras de Firewall
		_, err = compute.NewFirewall(ctx, "gameserver-operator-firewall-ingress", &compute.FirewallArgs{
			Network:      vpc.Name, // Atualizado para usar a nova VPC
			Direction:    pulumi.String("INGRESS"),
			SourceRanges: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			Allows: compute.FirewallAllowArray{
				&compute.FirewallAllowArgs{
					Protocol: pulumi.String("tcp"),
					Ports:    pulumi.StringArray{pulumi.String("0-65535")},
				},
				&compute.FirewallAllowArgs{
					Protocol: pulumi.String("udp"),
					Ports:    pulumi.StringArray{pulumi.String("0-65535")},
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = compute.NewFirewall(ctx, "gameserver-operator-firewall-egress", &compute.FirewallArgs{
			Network:           vpc.Name, // Atualizado para usar a nova VPC
			Direction:         pulumi.String("EGRESS"),
			DestinationRanges: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			Allows: compute.FirewallAllowArray{
				&compute.FirewallAllowArgs{
					Protocol: pulumi.String("tcp"),
					Ports:    pulumi.StringArray{pulumi.String("0-65535")},
				},
				&compute.FirewallAllowArgs{
					Protocol: pulumi.String("udp"),
					Ports:    pulumi.StringArray{pulumi.String("0-65535")},
				},
			},
		})
		if err != nil {
			return err
		}

		ctx.Export("clusterName", cluster.Name)
		ctx.Export("publicIP", ip.Address)

		return nil
	})
}

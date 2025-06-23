package main

import (
	"context"
	"flag"
	"log"
	"os"

	provider "github.com/appkins-org/terraform-provider-ironic/ironic"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
)

//go:generate terraform fmt -recursive ./examples/

const (
	ironicProviderName = "registry.terraform.io/appkins-org/ironic"
)

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate -provider-name ironic
func main() {
	ctx := context.Background()

	// Remove any date and time prefix in log package function output to
	// prevent duplicate timestamp and incorrect log level setting
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	debugFlag := flag.Bool("debug", false, "Start provider in debug mode.")
	flag.Parse()

	var serveOpts []tf6server.ServeOpt

	if *debugFlag {
		serveOpts = append(serveOpts, tf6server.WithManagedDebug())
	}

	upgradedSdkProvider, err := tf5to6server.UpgradeServer(
		context.Background(),
		provider.Provider().GRPCProvider,
	)
	if err != nil {
		tflog.Error(ctx, "Failed to upgrade SDK provider to protocol version 6",
			map[string]any{
				"error": err,
			},
		)
		os.Exit(1)
	}

	providers := []func() tfprotov6.ProviderServer{
		func() tfprotov6.ProviderServer {
			return upgradedSdkProvider
		},
		providerserver.NewProtocol6(provider.NewFrameworkProvider()),
	}

	muxServer, err := tf6muxserver.NewMuxServer(ctx, providers...)
	if err != nil {
		tflog.Error(ctx, "Failed to create MuxServer", map[string]any{
			"error": err,
		})
		os.Exit(1)
	}

	err = tf6server.Serve(ironicProviderName, muxServer.ProviderServer, serveOpts...)
	if err != nil {
		tflog.Error(ctx, "Failed to start serving the ProviderServer", map[string]any{
			"error": err,
		})
		os.Exit(1)
	}
}

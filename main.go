package main

import (
	"context"
	"flag"
	"log"

	provider "github.com/appkins-org/terraform-provider-ironic/ironic"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

//go:generate go tool github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate -provider-name ironic
func main() {
	var debug bool

	flag.BoolVar(
		&debug,
		"debug",
		false,
		"set to true to run the provider with support for debuggers like delve",
	)
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/appkins-org/ironic",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}

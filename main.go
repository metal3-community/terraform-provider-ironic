package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/metal3-community/terraform-provider-ironic/ironic"
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
		Address: "registry.terraform.io/metal3-community/ironic",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), ironic.New(), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}

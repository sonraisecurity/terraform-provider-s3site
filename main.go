package main

import (
	"github.com/hashicorp/terraform/plugin"
	"github.com/sonraisecurity/terraform-provider-s3site/s3site"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: s3site.Provider,
	})
}

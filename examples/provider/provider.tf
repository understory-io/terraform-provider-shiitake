terraform {
  required_providers {
    shiitake = {
      source = "understory/shiitake"
    }
  }
}

# Reads SHIITAKE_SERVER and SHIITAKE_SERVICE_ACCOUNT_API_KEY from the
# environment. The service-account key is Terraform-generated into AWS Secrets
# Manager (data/shiitake_server/env), so read it from there rather than pasting
# it into this block.
provider "shiitake" {}

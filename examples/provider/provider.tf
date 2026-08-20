terraform {
  required_providers {
    shiitake = {
      source = "understory-io/shiitake"
    }
  }
}

# Reads SHIITAKE_SERVER and SHIITAKE_SERVICE_ACCOUNT_API_KEY from the
# environment. The service-account key is Terraform-generated into a secret
# store, so read it from there rather than pasting it into this block.
provider "shiitake" {}

terraform {
  required_version = ">= 1.5"

  required_providers {
    newrelic = {
      source  = "newrelic/newrelic"
      version = "~> 3.44"
    }
  }
}

# account_id, api_key e region também podem vir das variáveis de ambiente
# NEW_RELIC_ACCOUNT_ID, NEW_RELIC_API_KEY e NEW_RELIC_REGION — o provider as lê
# automaticamente se as variáveis do Terraform ficarem sem valor. Nunca comitar
# um terraform.tfvars com api_key preenchida.
provider "newrelic" {
  account_id = var.newrelic_account_id
  api_key    = var.newrelic_api_key
  region     = var.newrelic_region
}

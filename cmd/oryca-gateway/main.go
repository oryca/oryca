// Command oryca-gateway is the data plane: it authenticates callers, applies the
// rate limits their package allows, and proxies what is left to the upstream API.
//
// It reads its configuration from the environment (see gateway/.env.example) and
// keeps its routing table in sync with the control plane over the contract in
// docs/sync-contract.md. It never opens a database connection of its own.
package main

import "github.com/oryca/oryca/gateway/app"

func main() {
	app.Run()
}

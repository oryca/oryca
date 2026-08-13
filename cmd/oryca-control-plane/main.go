// Command oryca-control-plane is the management side: the admin and portal API,
// the seeded configuration, and the consumer that turns the gateway's request
// stream into the data behind the dashboard.
//
// It reads its configuration from the environment (see control-plane/.env.example)
// and owns the only connection to MongoDB.
package main

import "github.com/oryca/oryca/control-plane/app"

func main() {
	app.Run()
}

package main

import (
	"log"
	"net/http"
	"strings"

	"clara-code/examples/internal/config"
	"clara-code/examples/internal/demo"
	relayconnector "clara-code/pkg/model/relay"
)

func main() {
	upstreamURL := strings.TrimSpace(config.Env("UPSTREAM_URL"))
	closeUpstream := func() {}
	if upstreamURL == "" {
		upstream := demo.ResponsesUpstream("Relay demo response.")
		upstreamURL = upstream.URL + "/v1"
		closeUpstream = upstream.Close
	}
	defer closeUpstream()
	protocol := relayconnector.ProtocolResponses
	if config.Env("UPSTREAM_PROTOCOL") == "completions" {
		protocol = relayconnector.ProtocolCompletions
	}
	handler, err := relayconnector.New(relayconnector.Config{
		Upstreams: map[string]relayconnector.Upstream{
			"default": {
				BaseURL: upstreamURL, APIKey: config.Env("UPSTREAM_API_KEY"),
				Protocol: protocol,
			},
		},
		Default: "default",
	})
	if err != nil {
		log.Fatal(err)
	}
	addr := config.Env("RELAY_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	log.Printf("Clara Core Relay listening on http://%s/v1", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

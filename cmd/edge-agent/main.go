package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wraplink/edge-agent/internal/agent"
	"github.com/wraplink/edge-agent/internal/allocator"
	"github.com/wraplink/edge-agent/internal/client"
	"github.com/wraplink/edge-agent/internal/config"
	"github.com/wraplink/edge-agent/internal/dataplane"
)

const (
	sniCapacity   = 5000
	routeCapacity = 1000
)

func main() {
	configFile := flag.String(
		"config",
		"config.yaml",
		"path to configuration file",
	)

	flag.Parse()

	/*
	 * Load configuration.
	 */
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf(
			"load config: %v",
			err,
		)
	}

	log.Printf(
		"edge-agent starting id=%s",
		cfg.Agent.ID,
	)

	log.Printf(
		"dns-control: %s",
		cfg.DNSControl.URL,
	)

	log.Printf(
		"poll interval: %s",
		cfg.Agent.Interval,
	)

	log.Printf(
		"edge addresses: %v",
		cfg.Edge.Addresses,
	)

	log.Printf(
		"capacity: sni=%d route=%d",
		sniCapacity,
		routeCapacity,
	)

	/*
	 * DNS-control API client.
	 */
	dnsClient := client.New(
		cfg.DNSControl.URL,
		cfg.DNSControl.Token,
	)

	/*
	 * SNI allocator.
	 *
	 * It uses the addresses configured for this edge.
	 */
	sniAllocator := allocator.NewSNI(
		cfg.Edge.Addresses,
	)

	/*
	 * Route allocator.
	 *
	 * It uses the same edge addresses for now.
	 */
	routeAllocator := allocator.NewRoute(
		cfg.Edge.Addresses,
	)

	/*
	 * nftables dataplane manager.
	 */
	dataplaneManager, err := dataplane.NewManager()
	if err != nil {
		log.Fatalf(
			"create dataplane manager: %v",
			err,
		)
	}

	/*
	 * Create edge agent.
	 */
	service := agent.New(
		cfg.Agent.ID,
		cfg.Agent.Interval,

		sniCapacity,
		routeCapacity,

		dnsClient,

		sniAllocator,
		routeAllocator,

		dataplaneManager,
	)

	/*
	 * Handle SIGINT / SIGTERM.
	 */
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	/*
	 * Run agent.
	 */
	if err := service.Run(ctx); err != nil {
		if ctx.Err() != nil {
			log.Printf(
				"edge-agent stopped",
			)

			return
		}

		log.Fatalf(
			"edge-agent failed: %v",
			err,
		)
	}
}

package main

import (
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/your-username/entra-exporter/config"
	"github.com/your-username/entra-exporter/collector"
	"net/http"
)

const (
	Author  = "Your Name"
	Version = "0.1.0"
)

var (
	argparser *flags.Parser
	opts      config.Opts
)

var logger = logrus.New()

func main() {
	initArgparser()

	logger.Infof("Starting Entra ID exporter v%s", Version)

	// Init config
	cfg := config.NewConfig(logger)
	if opts.Config != "" {
		if err := cfg.LoadConfigFile(opts.Config); err != nil {
			logger.Fatalf("Failed to load config file: %v", err)
		}
	}

	registry := prometheus.NewRegistry()

	// Set up collectors
	if cfg.Collector.General.IsEnabled() {
		generalCollector := collector.NewGeneralCollector(cfg, logger.WithField("collector", "general"))
		registry.MustRegister(generalCollector)
		logger.Info("Enabled collector: general")
	}

	if cfg.Collector.Users.IsEnabled() {
		usersCollector := collector.NewUsersCollector(cfg, logger.WithField("collector", "users"))
		registry.MustRegister(usersCollector)
		logger.Info("Enabled collector: users")
	}

	if cfg.Collector.Devices.IsEnabled() {
		devicesCollector := collector.NewDevicesCollector(cfg, logger.WithField("collector", "devices"))
		registry.MustRegister(devicesCollector)
		logger.Info("Enabled collector: devices")
	}

	/* Not yet implemented - will be added in future versions
	if cfg.Collector.Applications.IsEnabled() {
		applicationsCollector := collector.NewApplicationsCollector(cfg, logger.WithField("collector", "applications"))
		registry.MustRegister(applicationsCollector)
		logger.Info("Enabled collector: applications")
	}

	if cfg.Collector.ServicePrincipals.IsEnabled() {
		spCollector := collector.NewServicePrincipalsCollector(cfg, logger.WithField("collector", "servicePrincipals"))
		registry.MustRegister(spCollector)
		logger.Info("Enabled collector: servicePrincipals")
	}

	if cfg.Collector.Groups.IsEnabled() {
		groupsCollector := collector.NewGroupsCollector(cfg, logger.WithField("collector", "groups"))
		registry.MustRegister(groupsCollector)
		logger.Info("Enabled collector: groups")
	}

	if cfg.Collector.ConditionalAccessPolicies.IsEnabled() {
		capCollector := collector.NewConditionalAccessPoliciesCollector(cfg, logger.WithField("collector", "conditionalAccessPolicies"))
		registry.MustRegister(capCollector)
		logger.Info("Enabled collector: conditionalAccessPolicies")
	}

	if cfg.Collector.DirectoryRoles.IsEnabled() {
		rolesCollector := collector.NewDirectoryRolesCollector(cfg, logger.WithField("collector", "directoryRoles"))
		registry.MustRegister(rolesCollector)
		logger.Info("Enabled collector: directoryRoles")
	}
	*/

	// Start HTTP server
	startHttpServer(registry)
}

func initArgparser() {
	argparser = flags.NewParser(&opts, flags.Default)
	_, err := argparser.Parse()

	// check if there is an parse error
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println()
			argparser.WriteHelp(os.Stdout)
			os.Exit(1)
		}
	}

	// Configure logger
	if opts.Logger.Debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	if opts.Logger.Json {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}
}

func startHttpServer(registry *prometheus.Registry) {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Metrics endpoint
	handler := promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{
			Registry: registry,
		},
	)
	mux.Handle("/metrics", handler)

	// Root endpoint - show basic info
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<html>
			<head>
				<title>Entra ID Exporter</title>
			</head>
			<body>
				<h1>Entra ID Exporter</h1>
				<p><a href="/metrics">Metrics</a></p>
				<p><a href="/health">Health</a></p>
				<p><a href="https://github.com/your-username/entra-exporter">GitHub</a></p>
			</body>
			</html>
		`))
	})

	server := &http.Server{
		Addr:         opts.Server.Bind,
		Handler:      mux,
		ReadTimeout:  opts.Server.ReadTimeout,
		WriteTimeout: opts.Server.WriteTimeout,
	}

	logger.Infof("Starting HTTP server on %s", opts.Server.Bind)
	if err := server.ListenAndServe(); err != nil {
		logger.Fatal(err)
	}
}

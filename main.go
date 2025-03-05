package main

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/your-username/entra-exporter/config"
	"github.com/your-username/entra-exporter/collector"
)

const (
	// Version is the current version of this application
	Version = "0.1.0"
)

var (
	argparser *flags.Parser
	opts struct {
		Config         string `short:"c" long:"config" description:"Path to config file" default:"config.yml"`
		LogFile        string `short:"l" long:"log.file" description:"Log output file"`
		LogFormat      string `short:"f" long:"log.format" description:"Log format" choice:"text" choice:"json" default:"text"`
		LogLevel       string `short:"v" long:"log.level" description:"Log level" choice:"debug" choice:"info" choice:"warn" choice:"error" default:"info"`
		LogDebug       bool   `long:"log.debug" description:"Enable debug logging"`
		ListenAddress  string `short:"a" long:"web.listen-address" description:"Address to listen on for web interface and telemetry" default:":8080"`
	}
	logger = logrus.New()
)

func main() {
	initArgparser()

	logger.Infof("Starting Entra ID exporter v%s", Version)

	// Check for required environment variables for Azure authentication
	logger.Debug("Checking Azure authentication environment variables")
	azureTenant := os.Getenv("AZURE_TENANT_ID")
	azureClientID := os.Getenv("AZURE_CLIENT_ID")
	azureClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	
	if azureTenant == "" {
		logger.Warn("AZURE_TENANT_ID environment variable is not set. Using default tenant from Azure SDK.")
	}
	
	if azureClientID == "" {
		logger.Warn("AZURE_CLIENT_ID environment variable is not set. Authentication may fail if not using managed identity.")
	}
	
	if azureClientSecret == "" && azureClientID != "" {
		logger.Warn("AZURE_CLIENT_SECRET environment variable is not set but AZURE_CLIENT_ID is set. Authentication may fail.")
	}

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

	// Create HTTP server and metrics handler
	handler := promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{
			ErrorLog:      stdlog.New(logger.Writer(), "", 0),
			ErrorHandling: promhttp.ContinueOnError,
		},
	)

	// Add global recovery middleware to prevent crashes
	recoveryHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("Recovered from panic in HTTP handler: %v", r)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		handler.ServeHTTP(w, r)
	})

	// Register handlers
	http.Handle("/metrics", recoveryHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		<html>
		<head><title>Entra ID Exporter</title></head>
		<body>
		<h1>Entra ID Exporter</h1>
		<p><a href="/metrics">Metrics</a></p>
		</body>
		</html>
		`))
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	// Add a debug endpoint to check environment variables
	if logger.Level == logrus.DebugLevel {
		http.HandleFunc("/debug/env", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Azure Authentication Environment Variables:")
			fmt.Fprintln(w, "AZURE_TENANT_ID: ", maskValue(os.Getenv("AZURE_TENANT_ID")))
			fmt.Fprintln(w, "AZURE_CLIENT_ID: ", maskValue(os.Getenv("AZURE_CLIENT_ID")))
			fmt.Fprintln(w, "AZURE_CLIENT_SECRET: ", "[MASKED]")
		})
	}

	// Set up graceful shutdown
	server := &http.Server{
		Addr:    opts.ListenAddress,
		Handler: http.DefaultServeMux,
	}

	// Make a channel to listen for OS signals for graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Infof("Starting HTTP server on %s", opts.ListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("Error starting HTTP server: %v", err)
			os.Exit(1)
		}
	}()

	// Block until we receive a termination signal
	<-done
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Errorf("Server shutdown failed: %v", err)
	}

	logger.Info("Server gracefully stopped")
}

func initArgparser() {
	// Parse environment variables
	if os.Getenv("LOG_DEBUG") == "true" {
		opts.LogDebug = true
	}

	// Parse command line arguments
	argparser = flags.NewParser(&opts, flags.Default)
	if _, err := argparser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Printf("Error parsing arguments: %s\n", err)
			os.Exit(1)
		}
	}

	// Configure logger
	if opts.LogDebug {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		switch opts.LogLevel {
		case "debug":
			logger.SetLevel(logrus.DebugLevel)
		case "info":
			logger.SetLevel(logrus.InfoLevel)
		case "warn":
			logger.SetLevel(logrus.WarnLevel)
		case "error":
			logger.SetLevel(logrus.ErrorLevel)
		default:
			logger.SetLevel(logrus.InfoLevel)
		}
	}

	if opts.LogFormat == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	// Set log output
	if opts.LogFile != "" {
		file, err := os.OpenFile(opts.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Printf("Error opening log file: %s\n", err)
			os.Exit(1)
		}
		logger.SetOutput(file)
	}
}

// maskValue masks part of a string value for logging
func maskValue(value string) string {
	if value == "" {
		return "<not set>"
	}
	if len(value) <= 8 {
		return "****" + value[len(value)-4:]
	}
	return value[:4] + "****" + value[len(value)-4:]
}

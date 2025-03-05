package config

import "time"

// Opts is the structure of command line options
type Opts struct {
	// Logger options
	Logger struct {
		Debug bool `long:"log.debug" env:"LOG_DEBUG" description:"debug mode"`
		Json  bool `long:"log.json" env:"LOG_JSON" description:"Switch log output to json format"`
	} `group:"Logger Settings"`

	// Config file options
	Config string `long:"config" env:"CONFIG" description:"Path to config file"`

	// Azure options
	Azure struct {
		TenantID     string `long:"azure.tenant" env:"AZURE_TENANT_ID" description:"Azure tenant id"`
		Environment  string `long:"azure.environment" env:"AZURE_ENVIRONMENT" description:"Azure environment name" default:"AZUREPUBLICCLOUD"`
	} `group:"Azure Options"`

	// Cache options
	Cache struct {
		Path string `long:"cache.path" env:"CACHE_PATH" description:"Cache path (to folder, file://path...)"`
	} `group:"Cache Options"`

	// Server options
	Server struct {
		Bind         string        `long:"server.bind" env:"SERVER_BIND" description:"Server address" default:":8080"`
		ReadTimeout  time.Duration `long:"server.timeout.read" env:"SERVER_TIMEOUT_READ" description:"Server read timeout" default:"5s"`
		WriteTimeout time.Duration `long:"server.timeout.write" env:"SERVER_TIMEOUT_WRITE" description:"Server write timeout" default:"10s"`
	} `group:"Server Options"`
}

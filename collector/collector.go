package collector

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	mgraph "github.com/microsoftgraph/msgraph-sdk-go"
	graphauth "github.com/microsoft/kiota-authentication-azure-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/your-username/entra-exporter/config"
)

// BaseCollector is a base collector for all Microsoft Graph collectors
type BaseCollector struct {
	sync.Mutex

	name       string
	logger     *logrus.Entry
	config     *config.Config
	scrapeTime time.Duration

	graphClients      map[string]*mgraph.GraphServiceClient
	graphClientsLock  sync.RWMutex

	// Common metrics
	scrapeErrors *prometheus.CounterVec
	scrapeDuration *prometheus.SummaryVec
	lastScrapeTime *prometheus.GaugeVec
}

// NewBaseCollector creates a new base collector
func NewBaseCollector(name string, scrapeTime time.Duration, config *config.Config, logger *logrus.Entry) *BaseCollector {
	c := &BaseCollector{
		name:              name,
		logger:            logger,
		config:            config,
		scrapeTime:        scrapeTime,
		graphClients:      map[string]*mgraph.GraphServiceClient{},
		graphClientsLock:  sync.RWMutex{},
		
		scrapeErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: fmt.Sprintf("entraid_%s_scrape_errors_total", name),
				Help: fmt.Sprintf("Total number of Entra ID %s scrape errors", name),
			},
			[]string{"tenant_id"},
		),
		scrapeDuration: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name: fmt.Sprintf("entraid_%s_scrape_duration_seconds", name),
				Help: fmt.Sprintf("Duration of Entra ID %s scrape in seconds", name),
			},
			[]string{"tenant_id"},
		),
		lastScrapeTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("entraid_%s_last_scrape_time", name),
				Help: fmt.Sprintf("Last Entra ID %s scrape time in seconds since epoch", name),
			},
			[]string{"tenant_id"},
		),
	}

	return c
}

// GetGraphClient returns a Microsoft Graph client for a tenant
func (c *BaseCollector) GetGraphClient(tenantID string) (*mgraph.GraphServiceClient, error) {
	c.graphClientsLock.RLock()
	if client, exists := c.graphClients[tenantID]; exists {
		c.graphClientsLock.RUnlock()
		return client, nil
	}
	c.graphClientsLock.RUnlock()

	// Create a new client
	c.graphClientsLock.Lock()
	defer c.graphClientsLock.Unlock()

	// Double check to avoid race conditions
	if client, exists := c.graphClients[tenantID]; exists {
		return client, nil
	}

	c.logger.Debugf("Creating new Graph client for tenant: %s", tenantID)

	// Check if environment variables are set
	azureClientID := os.Getenv("AZURE_CLIENT_ID")
	azureClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	if tenantID == "" {
		// If tenant is empty, use the one from the environment
		tenantID = os.Getenv("AZURE_TENANT_ID")
		c.logger.Debugf("Using tenant ID from environment: %s", tenantID)
	}

	// Add more detailed logging about authentication method
	if azureClientID != "" && azureClientSecret != "" {
		c.logger.Debugf("Using client credentials flow for authentication (AZURE_CLIENT_ID and AZURE_CLIENT_SECRET)")
	} else if azureClientID != "" {
		c.logger.Debugf("Using client ID without secret (AZURE_CLIENT_ID only)")
	} else {
		c.logger.Debugf("Using default Azure credential chain (managed identity or other method)")
	}

	// Create a credential using the default Azure credential chain
	credOptions := &azidentity.DefaultAzureCredentialOptions{}
	if tenantID != "" {
		credOptions.TenantID = tenantID
	}
	
	cred, err := azidentity.NewDefaultAzureCredential(credOptions)
	if err != nil {
		c.logger.Errorf("Failed to create Azure credential: %v", err)
		c.logger.Debug("Authentication error details: Check if AZURE_TENANT_ID, AZURE_CLIENT_ID, and AZURE_CLIENT_SECRET environment variables are set correctly")
		return nil, fmt.Errorf("failed to create credential: %v", err)
	}

	// Try to validate the credential by getting a token
	c.logger.Debug("Validating Azure credential by requesting a token")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// The Microsoft Graph scope
	scopes := []string{"https://graph.microsoft.com/.default"}
	tokenRequestOptions := policy.TokenRequestOptions{
		Scopes: scopes,
	}
	_, err = cred.GetToken(ctx, tokenRequestOptions)
	if err != nil {
		c.logger.Errorf("Failed to validate Azure credential: %v", err)
		c.logger.Debug("Token acquisition failed: This usually indicates incorrect credentials or insufficient permissions")
		return nil, fmt.Errorf("failed to validate credential: %v", err)
	}
	c.logger.Debug("Successfully acquired authentication token")

	// Create an auth provider using the credential
	authProvider, err := graphauth.NewAzureIdentityAuthenticationProvider(cred)
	if err != nil {
		c.logger.Errorf("Failed to create auth provider: %v", err)
		return nil, fmt.Errorf("failed to create auth provider: %v", err)
	}

	// Create a request adapter
	adapter, err := mgraph.NewGraphRequestAdapter(authProvider)
	if err != nil {
		c.logger.Errorf("Failed to create adapter: %v", err)
		return nil, fmt.Errorf("failed to create adapter: %v", err)
	}

	c.logger.Debugf("Successfully created Graph client for tenant: %s", tenantID)

	// Create a Graph client
	client := mgraph.NewGraphServiceClient(adapter)
	c.graphClients[tenantID] = client

	return client, nil
}

// GetTenants returns a list of tenants from the config
func (c *BaseCollector) GetTenants() []string {
	tenants := c.config.Azure.Tenants
	
	// If no tenants are specified, use the one from the environment
	if len(tenants) == 0 {
		envTenant := os.Getenv("AZURE_TENANT_ID")
		if envTenant != "" {
			c.logger.Debugf("No tenants specified in config, using tenant from environment: %s", envTenant)
			tenants = []string{envTenant}
		} else {
			c.logger.Warn("No tenant IDs specified in config or environment variables. Authentication may fail or use default tenant.")
			// Adding empty string for default tenant in Azure SDK
			tenants = []string{""}
		}
	}
	
	c.logger.Debugf("Using tenants: %v", tenants)
	return tenants
}

// Describe implements prometheus.Collector
func (c *BaseCollector) Describe(ch chan<- *prometheus.Desc) {
	c.scrapeErrors.Describe(ch)
	c.scrapeDuration.Describe(ch)
	c.lastScrapeTime.Describe(ch)
}

// Collect implements prometheus.Collector
func (c *BaseCollector) Collect(ch chan<- prometheus.Metric) {
	c.scrapeErrors.Collect(ch)
	c.scrapeDuration.Collect(ch)
	c.lastScrapeTime.Collect(ch)
}

// StartCacheInvalidator starts background cache invalidation based on scrape time
func (c *BaseCollector) StartCacheInvalidator(collect func()) {
	go func() {
		c.logger.Infof("Starting cache invalidator for %s collector", c.name)
		
		// Recover from panics to keep the application running
		defer func() {
			if r := recover(); r != nil {
				c.logger.Errorf("PANIC in %s collector: %v", c.name, r)
				// Restart the goroutine after a short delay
				time.Sleep(5 * time.Second)
				c.logger.Infof("Restarting cache invalidator for %s collector after panic", c.name)
				// Restart the cache invalidator
				c.StartCacheInvalidator(collect)
			}
		}()

		for {
			// Wrap collection in another recover to prevent panics during each collection cycle
			func() {
				defer func() {
					if r := recover(); r != nil {
						c.logger.Errorf("PANIC during %s collection: %v", c.name, r)
					}
				}()
				
				// Run the collection
				c.logger.Debugf("Starting collection cycle for %s", c.name)
				collect()
				c.logger.Debugf("Completed collection cycle for %s", c.name)
			}()

			// Wait for next scrape
			c.logger.Debugf("Waiting %s for next %s collection cycle", c.scrapeTime, c.name)
			time.Sleep(c.scrapeTime)
		}
	}()
}

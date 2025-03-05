package collector

import (
	"fmt"
	"sync"
	"time"

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

	// Create a credential using the default Azure credential chain
	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %v", err)
	}

	// Create an auth provider using the credential
	authProvider, err := graphauth.NewAzureIdentityAuthenticationProvider(cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth provider: %v", err)
	}

	// Create a request adapter
	adapter, err := mgraph.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %v", err)
	}

	// Create a Graph client
	client := mgraph.NewGraphServiceClient(adapter)
	c.graphClients[tenantID] = client

	return client, nil
}

// GetTenants returns the list of tenants to scrape
func (c *BaseCollector) GetTenants() []string {
	// If tenants are explicitly configured, use them
	if len(c.config.Azure.Tenants) > 0 {
		return c.config.Azure.Tenants
	}

	// Otherwise, use the default tenant from environment
	return []string{""}
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
		for {
			// Initial collection
			collect()

			// Wait for next scrape
			time.Sleep(c.scrapeTime)
		}
	}()
}

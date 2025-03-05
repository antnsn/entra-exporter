package collector

import (
	"context"
	"sync"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/your-username/entra-exporter/config"
)

// UsersCollector collects Entra ID user metrics
type UsersCollector struct {
	*BaseCollector

	// Users cache
	usersLock sync.RWMutex
	usersList map[string][]models.Userable

	// Metrics
	usersTotal *prometheus.GaugeVec
	usersInfo  *prometheus.GaugeVec
}

// NewUsersCollector creates a new UsersCollector
func NewUsersCollector(config *config.Config, logger *logrus.Entry) *UsersCollector {
	scrapeTime := config.Collector.Users.ScrapeTime

	c := &UsersCollector{
		BaseCollector: NewBaseCollector("users", scrapeTime, config, logger),
		usersList:     map[string][]models.Userable{},
		usersTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "entraid_users_total",
				Help: "Total number of users in Entra ID",
			},
			[]string{"tenant_id"},
		),
		usersInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "entraid_users_info",
				Help: "Information about users in Entra ID",
			},
			[]string{
				"tenant_id",
				"user_id",
				"user_principal_name",
				"display_name",
				"account_enabled",
				"user_type",
				"creation_type",
			},
		),
	}

	// Start background collection
	c.StartCacheInvalidator(c.collect)

	return c
}

// Describe implements prometheus.Collector
func (c *UsersCollector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)
	c.usersTotal.Describe(ch)
	c.usersInfo.Describe(ch)
}

// Collect implements prometheus.Collector
func (c *UsersCollector) Collect(ch chan<- prometheus.Metric) {
	c.BaseCollector.Collect(ch)

	c.usersLock.RLock()
	defer c.usersLock.RUnlock()

	// Collect users metrics
	for tenantID, usersList := range c.usersList {
		c.usersTotal.WithLabelValues(tenantID).Set(float64(len(usersList)))

		for _, user := range usersList {
			accountEnabled := "false"
			if user.GetAccountEnabled() != nil && *user.GetAccountEnabled() {
				accountEnabled = "true"
			}

			userType := "unknown"
			if user.GetUserType() != nil {
				userType = *user.GetUserType()
			}

			creationType := "unknown"
			if user.GetCreationType() != nil {
				creationType = *user.GetCreationType()
			}

			c.usersInfo.WithLabelValues(
				tenantID,
				*user.GetId(),
				*user.GetUserPrincipalName(),
				*user.GetDisplayName(),
				accountEnabled,
				userType,
				creationType,
			).Set(1)
		}
	}

	c.usersTotal.Collect(ch)
	c.usersInfo.Collect(ch)
}

// collect gets all users
func (c *UsersCollector) collect() {
	c.Lock()
	defer c.Unlock()

	for _, tenantID := range c.GetTenants() {
		start := time.Now()
		c.logger.Debugf("Collecting users for tenant %s", tenantID)

		client, err := c.GetGraphClient(tenantID)
		if err != nil {
			c.logger.Errorf("Failed to get Graph client for tenant %s: %v", tenantID, err)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
			continue
		}

		// Set up pagination
		var usersList []models.Userable
		pageSize := int32(100)
		query := users.UsersRequestBuilderGetQueryParameters{
			Top: &pageSize,
			// Add select to limit the properties returned for each user to reduce API load
			Select: []string{"id", "userPrincipalName", "displayName", "accountEnabled", "userType", "creationType"},
		}

		// Optional: add filter if specified in config
		// if c.config.Collector.Users.Filter != "" {
		//   query.Filter = &c.config.Collector.Users.Filter
		// }

		reqConfig := users.UsersRequestBuilderGetRequestConfiguration{
			QueryParameters: &query,
		}

		// Get the first page
		result, err := client.Users().Get(context.Background(), &reqConfig)
		if err != nil {
			c.logger.Errorf("Failed to get users for tenant %s: %v", tenantID, err)
			c.logger.Debugf("API request details for users: tenantID=%s, filter=%s", 
				tenantID, c.config.Collector.Users.Filter)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
			continue
		}

		// Store the first page of users
		if result.GetValue() != nil {
			usersList = append(usersList, result.GetValue()...)
			c.logger.Debugf("Retrieved %d users in first page for tenant %s", len(result.GetValue()), tenantID)
		} else {
			c.logger.Warnf("No users returned in API response for tenant %s", tenantID)
		}

		// Handle pagination manually
		for result.GetOdataNextLink() != nil && len(*result.GetOdataNextLink()) > 0 {
			// Fetch the next page using the nextLink directly
			var nextReqConfig *users.UsersRequestBuilderGetRequestConfiguration
			result, err = client.Users().Get(context.Background(), nextReqConfig)
			if err != nil {
				c.logger.Errorf("Failed to get next page of users for tenant %s: %v", tenantID, err)
				c.scrapeErrors.WithLabelValues(tenantID).Inc()
				break
			}
			
			// Store this page's users
			if result.GetValue() != nil {
				usersList = append(usersList, result.GetValue()...)
			}
		}

		// Update the users list
		c.usersLock.Lock()
		c.usersList[tenantID] = usersList
		c.usersLock.Unlock()

		// Update scrape metrics
		duration := time.Since(start).Seconds()
		c.scrapeDuration.WithLabelValues(tenantID).Observe(duration)
		c.lastScrapeTime.WithLabelValues(tenantID).Set(float64(time.Now().Unix()))
		c.logger.Debugf("Completed users collection for tenant %s in %.2f seconds: %d users", tenantID, duration, len(usersList))
	}
}

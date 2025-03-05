package collector

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/your-username/entra-exporter/config"
)

// GeneralCollector collects general Entra ID metrics
type GeneralCollector struct {
	*BaseCollector

	// Stats cache
	statsLock sync.RWMutex
	stats     map[string]map[string]float64

	// Metrics
	statsMetric *prometheus.GaugeVec
}

// NewGeneralCollector creates a new GeneralCollector
func NewGeneralCollector(config *config.Config, logger *logrus.Entry) *GeneralCollector {
	scrapeTime := config.Collector.General.ScrapeTime

	c := &GeneralCollector{
		BaseCollector: NewBaseCollector("general", scrapeTime, config, logger),
		stats:         map[string]map[string]float64{},
		statsMetric: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "entraid_stats",
				Help: "Entra ID directory statistics",
			},
			[]string{"tenant_id", "metric"},
		),
	}

	// Start background collection
	c.StartCacheInvalidator(c.collect)

	return c
}

// Describe implements prometheus.Collector
func (c *GeneralCollector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)
	c.statsMetric.Describe(ch)
}

// Collect implements prometheus.Collector
func (c *GeneralCollector) Collect(ch chan<- prometheus.Metric) {
	c.BaseCollector.Collect(ch)

	c.statsLock.RLock()
	defer c.statsLock.RUnlock()

	// Collect stats metrics
	for tenantID, metrics := range c.stats {
		for metric, value := range metrics {
			c.statsMetric.WithLabelValues(tenantID, metric).Set(value)
		}
	}

	c.statsMetric.Collect(ch)
}

// collect gets all the general statistics
func (c *GeneralCollector) collect() {
	c.Lock()
	defer c.Unlock()

	for _, tenantID := range c.GetTenants() {
		start := time.Now()
		c.logger.Debugf("Collecting general metrics for tenant %s", tenantID)

		client, err := c.GetGraphClient(tenantID)
		if err != nil {
			c.logger.Errorf("Failed to get Graph client for tenant %s: %v", tenantID, err)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
			continue
		}

		// Create a new stats map for this tenant
		stats := make(map[string]float64)

		// Collect user count
		usersPage, err := client.Users().Get(context.Background(), nil)
		if err != nil {
			c.logger.Errorf("Failed to get users for tenant %s: %v", tenantID, err)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
		} else if usersPage != nil && usersPage.GetOdataCount() != nil {
			stats["user_count"] = float64(*usersPage.GetOdataCount())
		}

		// Collect device count
		devicesPage, err := client.Devices().Get(context.Background(), nil)
		if err != nil {
			c.logger.Errorf("Failed to get devices for tenant %s: %v", tenantID, err)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
		} else if devicesPage != nil && devicesPage.GetOdataCount() != nil {
			stats["device_count"] = float64(*devicesPage.GetOdataCount())
		}

		// Collect application count
		appsPage, err := client.Applications().Get(context.Background(), nil)
		if err != nil {
			c.logger.Errorf("Failed to get applications for tenant %s: %v", tenantID, err)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
		} else if appsPage != nil && appsPage.GetOdataCount() != nil {
			stats["application_count"] = float64(*appsPage.GetOdataCount())
		}

		// Collect service principal count
		spsPage, err := client.ServicePrincipals().Get(context.Background(), nil)
		if err != nil {
			c.logger.Errorf("Failed to get service principals for tenant %s: %v", tenantID, err)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
		} else if spsPage != nil && spsPage.GetOdataCount() != nil {
			stats["service_principal_count"] = float64(*spsPage.GetOdataCount())
		}

		// Collect group count
		groupsPage, err := client.Groups().Get(context.Background(), nil)
		if err != nil {
			c.logger.Errorf("Failed to get groups for tenant %s: %v", tenantID, err)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
		} else if groupsPage != nil && groupsPage.GetOdataCount() != nil {
			stats["group_count"] = float64(*groupsPage.GetOdataCount())
		}

		// Store the collected stats
		c.statsLock.Lock()
		c.stats[tenantID] = stats
		c.statsLock.Unlock()

		// Update scrape metrics
		duration := time.Since(start).Seconds()
		c.scrapeDuration.WithLabelValues(tenantID).Observe(duration)
		c.lastScrapeTime.WithLabelValues(tenantID).Set(float64(time.Now().Unix()))
		c.logger.Debugf("Completed general metrics collection for tenant %s in %.2f seconds", tenantID, duration)
	}
}

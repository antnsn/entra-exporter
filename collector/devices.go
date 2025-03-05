package collector

import (
	"context"
	"sync"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/devices"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/your-username/entra-exporter/config"
)

// DevicesCollector collects Entra ID device metrics
type DevicesCollector struct {
	*BaseCollector

	// Devices cache
	devicesLock sync.RWMutex
	devicesList map[string][]models.Deviceable

	// Metrics
	devicesTotal *prometheus.GaugeVec
	devicesInfo  *prometheus.GaugeVec
}

// NewDevicesCollector creates a new DevicesCollector
func NewDevicesCollector(config *config.Config, logger *logrus.Entry) *DevicesCollector {
	scrapeTime := config.Collector.Devices.ScrapeTime

	c := &DevicesCollector{
		BaseCollector: NewBaseCollector("devices", scrapeTime, config, logger),
		devicesList:     map[string][]models.Deviceable{},
		devicesTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "entraid_devices_total",
				Help: "Total number of devices in Entra ID",
			},
			[]string{"tenant_id"},
		),
		devicesInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "entraid_devices_info",
				Help: "Information about devices in Entra ID",
			},
			[]string{
				"tenant_id",
				"device_id",
				"display_name",
				"device_category",
				"operating_system",
				"operating_system_version",
				"trust_type",
				"enrollment_type",
				"account_enabled",
				"management_type",
				"ownership",
				"registration_datetime",
			},
		),
	}

	// Start background collection
	c.StartCacheInvalidator(c.collect)

	return c
}

// Describe implements prometheus.Collector
func (c *DevicesCollector) Describe(ch chan<- *prometheus.Desc) {
	c.BaseCollector.Describe(ch)
	c.devicesTotal.Describe(ch)
	c.devicesInfo.Describe(ch)
}

// Collect implements prometheus.Collector
func (c *DevicesCollector) Collect(ch chan<- prometheus.Metric) {
	c.BaseCollector.Collect(ch)

	c.devicesLock.RLock()
	defer c.devicesLock.RUnlock()

	// Collect devices metrics
	for tenantID, devicesList := range c.devicesList {
		c.devicesTotal.WithLabelValues(tenantID).Set(float64(len(devicesList)))

		for _, device := range devicesList {
			accountEnabled := "false"
			if device.GetAccountEnabled() != nil && *device.GetAccountEnabled() {
				accountEnabled = "true"
			}

			operatingSystem := "unknown"
			if device.GetOperatingSystem() != nil {
				operatingSystem = *device.GetOperatingSystem()
			}

			operatingSystemVersion := "unknown"
			if device.GetOperatingSystemVersion() != nil {
				operatingSystemVersion = *device.GetOperatingSystemVersion()
			}

			trustType := "unknown"
			if device.GetTrustType() != nil {
				trustType = *device.GetTrustType()
			}

			deviceCategory := "unknown"
			if device.GetDeviceCategory() != nil {
				deviceCategory = *device.GetDeviceCategory()
			}

			enrollmentType := "unknown"
			if device.GetEnrollmentType() != nil {
				enrollmentType = *device.GetEnrollmentType()
			}

			managementType := "unknown"
			if device.GetManagementType() != nil {
				managementType = *device.GetManagementType()
			}

			registrationDateTime := "unknown"
			if device.GetRegistrationDateTime() != nil {
				registrationDateTime = device.GetRegistrationDateTime().Format(time.RFC3339)
			}

			c.devicesInfo.WithLabelValues(
				tenantID,
				*device.GetId(),
				*device.GetDisplayName(),
				deviceCategory,
				operatingSystem,
				operatingSystemVersion,
				trustType,
				enrollmentType,
				accountEnabled,
				managementType,
				"n/a",
				registrationDateTime,
			).Set(1)
		}
	}

	c.devicesTotal.Collect(ch)
	c.devicesInfo.Collect(ch)
}

// collect gets all devices
func (c *DevicesCollector) collect() {
	c.Lock()
	defer c.Unlock()

	for _, tenantID := range c.GetTenants() {
		start := time.Now()
		c.logger.Debugf("Collecting devices for tenant %s", tenantID)

		client, err := c.GetGraphClient(tenantID)
		if err != nil {
			c.logger.Errorf("Failed to get Graph client for tenant %s: %v", tenantID, err)
			c.logger.Debugf("API request details for devices: tenantID=%s, filter=%s", 
				tenantID, c.config.Collector.Devices.Filter)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
			continue
		}

		// Set up pagination
		var devicesList []models.Deviceable
		pageSize := int32(100)
		query := devices.DevicesRequestBuilderGetQueryParameters{
			Top: &pageSize,
			// Add select to limit the properties returned for each device
			Select: []string{
				"id", "displayName", "operatingSystem", "operatingSystemVersion", 
				"accountEnabled", "trustType", "enrollmentType", "deviceCategory",
				"managementType", "registrationDateTime",
			},
		}

		// Optional: add filter if specified in config
		// if c.config.Collector.Devices.Filter != "" {
		//   query.Filter = &c.config.Collector.Devices.Filter
		// }

		reqConfig := devices.DevicesRequestBuilderGetRequestConfiguration{
			QueryParameters: &query,
		}

		// Get the first page
		result, err := client.Devices().Get(context.Background(), &reqConfig)
		if err != nil {
			c.logger.Errorf("Failed to get devices for tenant %s: %v", tenantID, err)
			c.logger.Debugf("API request details for devices: tenantID=%s, filter=%s", 
				tenantID, c.config.Collector.Devices.Filter)
			c.scrapeErrors.WithLabelValues(tenantID).Inc()
			continue
		}

		// Store the first page of devices
		if result.GetValue() != nil {
			devicesList = append(devicesList, result.GetValue()...)
			c.logger.Debugf("Retrieved %d devices in first page for tenant %s", len(result.GetValue()), tenantID)
		} else {
			c.logger.Warnf("No devices returned in API response for tenant %s", tenantID)
		}

		// Handle pagination manually
		for result.GetOdataNextLink() != nil && len(*result.GetOdataNextLink()) > 0 {
			// Fetch the next page using the nextLink directly
			var nextReqConfig *devices.DevicesRequestBuilderGetRequestConfiguration
			result, err = client.Devices().Get(context.Background(), nextReqConfig)
			if err != nil {
				c.logger.Errorf("Failed to get next page of devices for tenant %s: %v", tenantID, err)
				c.scrapeErrors.WithLabelValues(tenantID).Inc()
				break
			}
			
			// Store this page's devices
			if result.GetValue() != nil {
				devicesList = append(devicesList, result.GetValue()...)
			}
		}

		// Update the devices list
		c.devicesLock.Lock()
		c.devicesList[tenantID] = devicesList
		c.devicesLock.Unlock()

		// Update scrape metrics
		duration := time.Since(start).Seconds()
		c.scrapeDuration.WithLabelValues(tenantID).Observe(duration)
		c.lastScrapeTime.WithLabelValues(tenantID).Set(float64(time.Now().Unix()))
		c.logger.Debugf("Completed devices collection for tenant %s in %.2f seconds: %d devices", tenantID, duration, len(devicesList))
	}
}

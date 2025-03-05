# Entra ID Exporter

Prometheus exporter for Microsoft Entra ID (formerly Azure AD) metrics, such as:
- Number of users
- Number of devices
- Application registrations
- Service principals
- And more

## Features

- Uses the official [Microsoft Graph SDK for Go](https://github.com/microsoftgraph/msgraph-sdk-go)
- Supports all Azure environments (Azure public cloud, Azure government cloud, Azure China cloud, etc.)
- Docker image based on [Google's distroless](https://github.com/GoogleContainerTools/distroless) static image to reduce attack surface
- Can run non-root and with readonly root filesystem (security best practice)
- Publishes Azure API rate limit metrics

## Configuration

```
Usage:
  entra-exporter [OPTIONS]

Application Options:
      --log.debug             Debug mode [$LOG_DEBUG]
      --log.json              Switch log output to json format [$LOG_JSON]
      --config=               Path to config file [$CONFIG]
      --azure.tenant=         Azure tenant id [$AZURE_TENANT_ID]
      --azure.environment=    Azure environment name (default: AZUREPUBLICCLOUD) [$AZURE_ENVIRONMENT]
      --cache.path=           Cache path (to folder, file://path...) [$CACHE_PATH]
      --server.bind=          Server address (default: :8080) [$SERVER_BIND]
      --server.timeout.read=  Server read timeout (default: 5s) [$SERVER_TIMEOUT_READ]
      --server.timeout.write= Server write timeout (default: 10s) [$SERVER_TIMEOUT_WRITE]

Help Options:
  -h, --help                  Show this help message
```

For Azure API authentication (using ENV vars) see [Azure SDK for Go Authentication](https://docs.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication)

## Config file
See [example.yaml](example.yaml) for a sample configuration.

## Azure Permissions
This exporter needs the following Microsoft Graph API permissions:
- `User.Read.All` - For reading user information
- `Device.Read.All` - For reading device information
- `Application.Read.All` - For reading application information
- `Directory.Read.All` - For reading directory information

## Metrics

- `entraid_stats` - General statistics about the Entra ID directory
- `entraid_users_total` - Total number of users
- `entraid_users_info` - User information
- `entraid_devices_total` - Total number of devices
- `entraid_devices_info` - Device information
- `entraid_applications_total` - Total number of application registrations
- `entraid_applications_info` - Application information
- `entraid_serviceprincipal_info` - Service principal information
- `entraid_serviceprincipal_credential` - Service principal credential expiry
- `entraid_groups_total` - Total number of groups
- `entraid_groups_info` - Group information
- `entraid_conditional_access_policies_total` - Total number of conditional access policies
- `entraid_conditional_access_policies_info` - Conditional access policy information
- `entraid_directory_roles_total` - Total number of directory roles
- `entraid_directory_roles_info` - Directory role information

## Development

### Requirements
- Go >= 1.21

### Building
```
make build
```

package analyzers

import (
	"context"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cosmos/armcosmos"
)

type CosmosDBAnalyzer struct {
	diagnosticsSettings DiagnosticsSettings
	subscriptionId      string
	ctx                 context.Context
	cred                azcore.TokenCredential
	databasesClient     *armcosmos.DatabaseAccountsClient
}

func NewCosmosDBAnalyzer(subscriptionId string, ctx context.Context, cred azcore.TokenCredential) *CosmosDBAnalyzer {
	diagnosticsSettings, _ := NewDiagnosticsSettings(cred, ctx)
	databasesClient, err := armcosmos.NewDatabaseAccountsClient(subscriptionId, cred, nil)
	if err != nil {
		log.Fatal(err)
	}
	analyzer := CosmosDBAnalyzer{
		diagnosticsSettings: *diagnosticsSettings,
		subscriptionId:      subscriptionId,
		ctx:                 ctx,
		cred:                cred,
		databasesClient:     databasesClient,
	}
	return &analyzer
}

func (c CosmosDBAnalyzer) Review(resourceGroupName string) ([]AzureServiceResult, error) {
	log.Printf("Analyzing CosmosDB Databases in Resource Group %s", resourceGroupName)

	databases, err := c.listDatabases(resourceGroupName)
	if err != nil {
		return nil, err
	}
	results := []AzureServiceResult{}
	for _, database := range databases {
		hasDiagnostics, err := c.diagnosticsSettings.HasDiagnostics(*database.ID)
		if err != nil {
			return nil, err
		}

		sla := "99.99%"
		availabilityZones := false
		availabilityZonesNotEnabledInALocation := false
		numberOfLocations := 0
		for _, location := range database.Properties.Locations {
			numberOfLocations++
			if *location.IsZoneRedundant {
				availabilityZones = true
				sla = "99.995%"
			} else {
				availabilityZonesNotEnabledInALocation = true
			}
		}

		if availabilityZones && numberOfLocations >= 2 && !availabilityZonesNotEnabledInALocation {
			sla = "99.999%"
		}

		results = append(results, AzureServiceResult{
			SubscriptionId:     c.subscriptionId,
			ResourceGroup:      resourceGroupName,
			ServiceName:        *database.Name,
			Sku:                string(*database.Properties.DatabaseAccountOfferType),
			Sla:                sla,
			Type:               *database.Type,
			AvailabilityZones:  availabilityZones,
			PrivateEndpoints:   len(database.Properties.PrivateEndpointConnections) > 0,
			DiagnosticSettings: hasDiagnostics,
			CAFNaming:          strings.HasPrefix(*database.Name, "cosmos"),
		})
	}
	return results, nil
}

func (c CosmosDBAnalyzer) listDatabases(resourceGroupName string) ([]*armcosmos.DatabaseAccountGetResults, error) {
	pager := c.databasesClient.NewListByResourceGroupPager(resourceGroupName, nil)

	domains := make([]*armcosmos.DatabaseAccountGetResults, 0)
	for pager.More() {
		resp, err := pager.NextPage(c.ctx)
		if err != nil {
			return nil, err
		}
		domains = append(domains, resp.Value...)
	}
	return domains, nil
}

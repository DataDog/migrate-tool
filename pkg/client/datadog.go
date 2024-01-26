package client

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/migrate-tool/pkg/config"
)

func Datadog() *datadog.APIClient {
	configuration := datadog.NewConfiguration()
	configuration.RetryConfiguration = datadog.RetryConfiguration{
		EnableRetry: true,
		BackOffBase: 2,
	}

	return datadog.NewAPIClient(configuration)
}

func DatadogCredentials(ctx context.Context, cfg config.Config, orgID int) (context.Context, error) {
	creds, found := cfg.Credentials[strconv.Itoa(orgID)]
	if !found {
		return nil, fmt.Errorf("no credential from orgID: %d", orgID)
	}

	ctx = context.WithValue(
		ctx,
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: creds.APIKey,
			},
			"appKeyAuth": {
				Key: creds.AppKey,
			},
		},
	)
	return ctx, nil
}

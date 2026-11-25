# Octopus Deploy OpenFeature provider for Go

[![CI build](https://github.com/OctopusDeploy/openfeature-provider-go/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/OctopusDeploy/openfeature-provider-go/actions/workflows/main.yml?query=branch:main)

The Octopus Deploy [OpenFeature provider](https://openfeature.dev/docs/reference/concepts/provider/) for Go, to be used with the [OpenFeature Go SDK](https://openfeature.dev/docs/reference/technologies/server/go/).

This provider is for server-side applications. It isn't intended for clients such as browser or desktop applications.

## About Octopus Deploy

[Octopus Deploy](https://octopus.com) is a sophisticated, best-of-breed continuous delivery (CD) platform for modern software teams. Octopus offers powerful release orchestration, deployment automation, and runbook automation, while handling the scale, complexity and governance expectations of even the largest organizations with the most complex deployment challenges.

## Supported Go Versions

This provider supports Go 1.26 and above. It follows the minimum Go version of the OpenFeature Go SDK, so the minimum moves up when the SDK's does.

## Getting Started

### Installation

```bash
go get github.com/open-feature/go-sdk github.com/OctopusDeploy/openfeature-provider-go
```

### Usage

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	octopus "github.com/OctopusDeploy/openfeature-provider-go"
	"github.com/open-feature/go-sdk/openfeature"
)

func main() {
	productMetadata, err := octopus.NewProductMetadataWithVersion("YourProductName", "1.0.0")
	if err != nil {
		log.Fatal(err)
	}

	configuration := octopus.NewOctopusFeatureConfiguration(os.Getenv("OCTOPUS_FEATURE_FLAGS_CLIENT_IDENTIFIER"), productMetadata)

	provider, err := octopus.NewOctopusFeatureProvider(configuration)
	if err != nil {
		log.Fatal(err)
	}

	// Wait for the flags at startup, so your first evaluations don't get the default value.
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		log.Fatal(err)
	}
	defer openfeature.Shutdown()

	client := openfeature.NewDefaultClient()

	// Evaluation context is passed with each evaluation, so one provider serves every request.
	evaluationContext := openfeature.NewEvaluationContext("user-123", map[string]any{"region": "eu"})

	enabled, _ := client.BooleanValue(context.Background(), "to-the-moon-feature", false, evaluationContext)
	if enabled {
		fmt.Println("🚀🚀🚀")
	}
}
```

This example is compiled with the tests, in [`example_test.go`](example_test.go).

Octopus feature flags are boolean, so evaluate them with `BooleanValue`. `StringValue`, `FloatValue`, `IntValue` and `ObjectValue` return your default value with a `TYPE_MISMATCH` error code, or `FLAG_NOT_FOUND` if the flag doesn't exist.

Targeting rules match against string attributes in the evaluation context, and percentage rollouts use its targeting key. Without a targeting key, a caller is only included in a rollout set to 100%.

The provider fetches flags when it initializes. If it can't retrieve them, `SetProviderAndWait` still doesn't return an error. The provider becomes ready, and every evaluation returns your default value with a `FLAG_NOT_FOUND` error code until a fetch succeeds.

After that, it checks for changes every `CacheDuration` and fetches the flags again only when they have changed. If a check fails, the provider keeps evaluating against the flags it already has and tries again at the next interval.

`openfeature.Shutdown()` stops the polling and closes the idle connections of the HTTP client the provider created. It never closes an `HttpClient` you provide.

For information on using the OpenFeature client, refer to the [OpenFeature Go SDK documentation](https://openfeature.dev/docs/reference/technologies/server/go/).

### Configuration

`NewOctopusFeatureConfiguration` sets the required options and the defaults. Set any optional fields on the configuration before you pass it to `NewOctopusFeatureProvider`. The provider keeps its own copy, so later changes have no effect.

| Option | Required | Description |
| --- | --- | --- |
| `ClientIdentifier` | Yes | The client identifier from the Octopus variable `Octopus.FeatureFlags.ClientIdentifier`. |
| `ProductMetadata` | Yes | The name and, optionally, version of your application, sent to Octopus for telemetry. Create it with `NewProductMetadata` or `NewProductMetadataWithVersion`. |
| `CacheDuration` | No | How long between checks for changed flags. Defaults to 1 minute. Must be greater than 0. |
| `RequestTimeout` | No | How long a request to Octopus may take before it's abandoned. Defaults to 10 seconds. Must be greater than 0, and should be well under `CacheDuration`. |
| `ReleaseVersionOverride` | No | Overrides the release version embedded in the client identifier. |
| `Logger` | No | A `*slog.Logger` for the provider's own log messages. If it's nil, the provider logs nothing. |
| `HttpClient` | No | The `*http.Client` for requests to Octopus. If it's nil, the provider creates its own. |

Provide an `HttpClient` to add tracing, a proxy or custom TLS. For example, to trace requests with [OpenTelemetry](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp):

```go
configuration.HttpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) to build and test locally, and [RELEASING.md](RELEASING.md) for how the provider is released.

## License

[Apache 2.0](LICENSE)

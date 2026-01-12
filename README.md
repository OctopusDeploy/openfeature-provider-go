# Octopus Deploy OpenFeature Provider for Go

[![Build and Test](https://github.com/OctopusDeploy/openfeature-provider-go/actions/workflows/ci.yml/badge.svg)](https://github.com/OctopusDeploy/openfeature-provider-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/OctopusDeploy/openfeature-provider-go)](https://goreportcard.com/report/github.com/OctopusDeploy/openfeature-provider-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/OctopusDeploy/openfeature-provider-go.svg)](https://pkg.go.dev/github.com/OctopusDeploy/openfeature-provider-go)

The Octopus Deploy [OpenFeature provider](https://openfeature.dev/docs/reference/concepts/provider/) for Go.

## About Octopus Deploy

[Octopus Deploy](https://octopus.com) is a sophisticated, best-of-breed continuous delivery (CD) platform for modern software teams. Octopus offers powerful release orchestration, deployment automation, and runbook automation, while handling the scale, complexity and governance expectations of even the largest organizations with the most complex deployment challenges.

## Supported Go Versions

This SDK is built for Go 1.24 and above.

If you require support for additional versions, please raise an issue.

## Getting Started

### Installation

```bash
go get github.com/OctopusDeploy/openfeature-provider-go
```

### Usage

```go
package main

import (
	"context"
	"fmt"
	"os"

	octopus "github.com/OctopusDeploy/openfeature-provider-go"
	"github.com/open-feature/go-sdk/openfeature"
)

func main() {
	// Get client identifier from environment
	clientIdentifier := os.Getenv("Octopus__FeatureToggles__ClientIdentifier")

	// Create configuration with defaults
	config := octopus.NewOctopusConfiguration(clientIdentifier)

	// Create the Octopus provider
	provider := octopus.NewOctopusProvider(config)

	// Set the provider in OpenFeature
	openfeature.SetProviderAndWait(provider)

	// Get a client
	client := openfeature.NewClient("my-app")

	// Evaluate a feature flag
	value, err := client.BooleanValue(
		context.Background(),
		"my-feature",
		false,
		openfeature.EvaluationContext{},
	)

	if err != nil {
		fmt.Printf("Error evaluating feature: %v\n", err)
		return
	}

	if value {
		fmt.Println("🚀🚀🚀")
	}
}
```

### Configuration Options

You can customize the provider configuration:

```go
config := octopus.NewOctopusConfiguration(clientIdentifier).
	WithServerURI("https://custom.features.octopus.com").
	WithCacheDuration(30 * time.Second)

provider := octopus.NewOctopusProvider(config)
```

### Segment Targeting

Feature toggles can be targeted to specific segments using evaluation context:

```go
evalContext := openfeature.NewEvaluationContext(map[string]interface{}{
    "user-id": "12345",
	"license-type": "premium",
	"region":       "us-east",
})

value, err := client.BooleanValueDetails(
	context.Background(),
	"premium-feature",
	false,
	evalContext,
)
```

Read more about targeting segments in the [Octopus Deploy Feature Toggles documentation](https://octopus.com/docs/feature-toggles#segments).


## Documentation

For more information, please refer to the [OpenFeature Documentation](https://docs.openfeature.dev/docs/reference/concepts/evaluation-api/) and the [Octopus Deploy Feature Toggles documentation](https://octopus.com/docs/feature-toggles).


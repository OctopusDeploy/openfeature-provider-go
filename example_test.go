package octopus_test

import (
	"context"
	"fmt"
	"log"
	"os"

	octopus "github.com/OctopusDeploy/openfeature-provider-go"
	"github.com/open-feature/go-sdk/openfeature"
)

// The README's usage section is a copy of this example. Keep the two in step.
func Example() {
	productMetadata, err := octopus.NewProductMetadataWithVersion("YourProductName", "1.0.0")
	if err != nil {
		log.Fatal(err)
	}

	configuration := octopus.NewFeatureConfiguration(os.Getenv("OCTOPUS_FEATURE_FLAGS_CLIENT_IDENTIFIER"), productMetadata)

	provider, err := octopus.NewFeatureProvider(configuration)
	if err != nil {
		log.Fatal(err)
	}

	// Wait for the flags at startup, so your first evaluations don't get the default value.
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		log.Fatal(err)
	}
	defer openfeature.Shutdown()

	client := openfeature.NewDefaultClient()

	enabled, _ := client.BooleanValue(context.Background(), "to-the-moon-feature", false, openfeature.EvaluationContext{})
	if enabled {
		fmt.Println("🚀🚀🚀")
	}
}

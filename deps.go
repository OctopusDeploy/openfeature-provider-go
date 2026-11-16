package octopus

// Keeps go-sdk in go.mod until the provider imports it for real, so go mod
// tidy doesn't drop it. Delete this file once a real import exists.
import _ "github.com/open-feature/go-sdk/openfeature"

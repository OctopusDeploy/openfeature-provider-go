package octopus

import (
	"errors"
	"strings"
)

// ProductMetadata describes the product using the provider. The feature flags
// service receives it with each request, for telemetry.
type ProductMetadata struct {
	name    string
	version string
}

// NewProductMetadata returns metadata with the product's name only.
// Characters that aren't valid in an HTTP token are removed from name, and
// an error is returned if none remain.
func NewProductMetadata(name string) (ProductMetadata, error) {
	metadata := ProductMetadata{name: removeNonTokenChars(name)}
	if metadata.name == "" {
		return ProductMetadata{}, errEmptyProductName
	}

	return metadata, nil
}

// NewProductMetadataWithVersion returns metadata with the product's name and
// version. Characters that aren't valid in an HTTP token are removed from
// both, and an error is returned if none remain in either.
func NewProductMetadataWithVersion(name, version string) (ProductMetadata, error) {
	metadata := ProductMetadata{name: removeNonTokenChars(name), version: removeNonTokenChars(version)}

	var errs []error
	if metadata.name == "" {
		errs = append(errs, errEmptyProductName)
	}
	if metadata.version == "" {
		errs = append(errs, errors.New("product version must contain at least one valid token character"))
	}
	if len(errs) > 0 {
		return ProductMetadata{}, errors.Join(errs...)
	}

	return metadata, nil
}

// Name returns the product name.
func (m ProductMetadata) Name() string {
	return m.name
}

// Version returns the product version, or "" if none was given.
func (m ProductMetadata) Version() string {
	return m.version
}

var errEmptyProductName = errors.New("product name must contain at least one valid token character")

// removeNonTokenChars keeps only RFC 9110 tchars, so the value is safe to send
// in a header: https://www.rfc-editor.org/rfc/rfc9110.html#name-tokens
func removeNonTokenChars(value string) string {
	return strings.Map(func(r rune) rune {
		if isTokenChar(r) {
			return r
		}
		return -1
	}, value)
}

func isTokenChar(r rune) bool {
	return 'a' <= r && r <= 'z' ||
		'A' <= r && r <= 'Z' ||
		'0' <= r && r <= '9' ||
		strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

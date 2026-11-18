package octopus

// ProductMetadata describes the product using the provider. The feature flags
// service receives it with each request, for telemetry.
type ProductMetadata struct {
	name    string
	version string
}

// NewProductMetadata returns metadata with the product's name only.
func NewProductMetadata(name string) (ProductMetadata, error) {
	return ProductMetadata{name: name}, nil
}

// NewProductMetadataWithVersion returns metadata with the product's name and
// version.
func NewProductMetadataWithVersion(name, version string) (ProductMetadata, error) {
	return ProductMetadata{name: name, version: version}, nil
}

// Name returns the product name.
func (m ProductMetadata) Name() string {
	return m.name
}

// Version returns the product version, or "" if none was given.
func (m ProductMetadata) Version() string {
	return m.version
}

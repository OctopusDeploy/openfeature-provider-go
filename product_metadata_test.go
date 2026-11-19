package octopus

import (
	"strings"
	"testing"
)

func TestNewProductMetadataCleansName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid characters are unchanged", "OctopusDeploy", "OctopusDeploy"},
		{"unsupported characters are removed", "My ,Product (v2.0)/release@2024:final", "MyProductv2.0release2024final"},
		{"hyphens are preserved", "My-Product", "My-Product"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := NewProductMetadata(test.input)
			if err != nil {
				t.Fatal(err)
			}

			if metadata.Name() != test.want {
				t.Errorf("Name() = %q, want %q", metadata.Name(), test.want)
			}
			if metadata.Version() != "" {
				t.Errorf("Version() = %q, want empty", metadata.Version())
			}
		})
	}
}

func TestNewProductMetadataWithVersionCleansVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid characters are unchanged", "2024.1.0", "2024.1.0"},
		{"unsupported characters are removed", "2024.1 (beta)", "2024.1beta"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := NewProductMetadataWithVersion("MyProduct", test.input)
			if err != nil {
				t.Fatal(err)
			}

			if metadata.Version() != test.want {
				t.Errorf("Version() = %q, want %q", metadata.Version(), test.want)
			}
		})
	}
}

func TestProductMetadataConstructorsRejectValuesEmptyAfterCleaning(t *testing.T) {
	tests := []struct {
		name      string
		construct func() (ProductMetadata, error)
		wantErrs  []string
	}{
		{"name only", func() (ProductMetadata, error) { return NewProductMetadata("   ") }, []string{"product name"}},
		{"name with version", func() (ProductMetadata, error) { return NewProductMetadataWithVersion("   ", "1.0") }, []string{"product name"}},
		{"version", func() (ProductMetadata, error) { return NewProductMetadataWithVersion("MyProduct", "   ") }, []string{"product version"}},
		{"name and version", func() (ProductMetadata, error) { return NewProductMetadataWithVersion("   ", "   ") }, []string{"product name", "product version"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.construct()
			if err == nil {
				t.Fatal("expected an error")
			}

			for _, want := range test.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

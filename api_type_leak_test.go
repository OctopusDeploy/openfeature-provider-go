package octopus_test

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"
)

const openFeatureSdkPath = "github.com/open-feature/go-sdk"

func TestPublicApiExposesOnlyImportableTypes(t *testing.T) {
	for _, leak := range unimportableTypes(importPublicPackage(t)) {
		t.Errorf("%s, which consumers can't import", leak)
	}
}

func TestUnimportableTypesFindsLeaks(t *testing.T) {
	const internalSource = `
		package hidden

		type Evaluator struct{}`

	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name: "internal return value",
			source: `
				package m

				import "example.com/m/internal/hidden"

				func New() *hidden.Evaluator { return nil }`,
			want: []string{"New exposes example.com/m/internal/hidden.Evaluator"},
		},
		{
			name: "unexported field type",
			source: `
				package m

				type Config struct{ Options options }

				type options struct{}`,
			want: []string{"Config.Options exposes example.com/m.options"},
		},
		{
			name: "embedded internal type",
			source: `
				package m

				import "example.com/m/internal/hidden"

				type Provider struct{ *hidden.Evaluator }`,
			want: []string{"Provider.Evaluator exposes example.com/m/internal/hidden.Evaluator"},
		},
		{
			name: "alias of an internal type",
			source: `
				package m

				import "example.com/m/internal/hidden"

				type Evaluator = hidden.Evaluator`,
			want: []string{"Evaluator exposes example.com/m/internal/hidden.Evaluator"},
		},
		{
			name: "standard library, go-sdk and unexported fields",
			source: `
				package m

				import (
					"log/slog"

					"github.com/open-feature/go-sdk/openfeature"
				)

				type Config struct {
					Logger *slog.Logger
					state  state
				}

				type state struct{}

				func (Config) Hooks() []openfeature.Hook { return nil }`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixtures := &fixtureImporter{
				fset:     token.NewFileSet(),
				sources:  map[string]string{"example.com/m": test.source, "example.com/m/internal/hidden": internalSource},
				packages: map[string]*types.Package{},
			}
			fixtures.fallback = importer.ForCompiler(fixtures.fset, "source", nil)

			pkg, err := fixtures.Import("example.com/m")
			if err != nil {
				t.Fatal(err)
			}

			if got := unimportableTypes(pkg); !slices.Equal(got, test.want) {
				t.Errorf("unimportableTypes() = %q, want %q", got, test.want)
			}
		})
	}
}

// unimportableTypes reports the named types reachable from the package's
// exported API that a consumer can't name: types under an internal/ path,
// unexported types, and types from modules other than this one, the standard
// library and the OpenFeature SDK. It doesn't look inside other modules'
// types, which are theirs to keep importable.
func unimportableTypes(pkg *types.Package) []string {
	finder := &leakFinder{root: pkg.Path()}

	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}

		obj := scope.Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok {
			finder.walk(obj.Type(), name)
			continue
		}

		if alias, ok := typeName.Type().(*types.Alias); ok {
			finder.walk(alias.Rhs(), name)
			continue
		}

		named := typeName.Type().(*types.Named)
		finder.walk(named.Underlying(), name)
		for method := range named.Methods() {
			if method.Exported() {
				finder.walk(method.Type(), name+"."+method.Name())
			}
		}
	}

	return finder.leaks
}

type leakFinder struct {
	root  string
	leaks []string
}

func (f *leakFinder) walk(t types.Type, from string) {
	switch t := t.(type) {
	case *types.Pointer:
		f.walk(t.Elem(), from)
	case *types.Slice:
		f.walk(t.Elem(), from)
	case *types.Array:
		f.walk(t.Elem(), from)
	case *types.Chan:
		f.walk(t.Elem(), from)
	case *types.Map:
		f.walk(t.Key(), from)
		f.walk(t.Elem(), from)
	case *types.Signature:
		for param := range t.TypeParams().TypeParams() {
			f.walk(param.Constraint(), from)
		}
		for param := range t.Params().Variables() {
			f.walk(param.Type(), from)
		}
		for result := range t.Results().Variables() {
			f.walk(result.Type(), from)
		}
	case *types.Struct:
		// Embedded fields count even when unexported, because their exported
		// fields and methods are promoted.
		for field := range t.Fields() {
			if field.Exported() || field.Embedded() {
				f.walk(field.Type(), from+"."+field.Name())
			}
		}
	case *types.Interface:
		for method := range t.Methods() {
			if method.Exported() {
				f.walk(method.Type(), from+"."+method.Name())
			}
		}
		for embedded := range t.EmbeddedTypes() {
			f.walk(embedded, from)
		}
	case *types.Union:
		for term := range t.Terms() {
			f.walk(term.Type(), from)
		}
	case *types.TypeParam:
		f.walk(t.Constraint(), from)
	case *types.Alias:
		f.check(t.Obj(), from)
		for arg := range t.TypeArgs().Types() {
			f.walk(arg, from)
		}
	case *types.Named:
		f.check(t.Obj(), from)
		for arg := range t.TypeArgs().Types() {
			f.walk(arg, from)
		}
	}
}

// check needs no recursion into this package's own types, because
// unimportableTypes walks every exported declaration anyway.
func (f *leakFinder) check(obj *types.TypeName, from string) {
	if obj.Pkg() == nil {
		return // A predeclared type such as error
	}

	path := obj.Pkg().Path()
	importable := false
	switch {
	case slices.Contains(strings.Split(path, "/"), "internal"), !obj.Exported():
	case path == f.root, strings.HasPrefix(path, f.root+"/"):
		importable = true
	case path == openFeatureSdkPath, strings.HasPrefix(path, openFeatureSdkPath+"/"):
		importable = true
	case !strings.Contains(strings.Split(path, "/")[0], "."):
		importable = true // The standard library
	}

	leak := fmt.Sprintf("%s exposes %s.%s", from, path, obj.Name())
	if !importable && !slices.Contains(f.leaks, leak) {
		f.leaks = append(f.leaks, leak)
	}
}

// fixtureImporter type-checks packages from source strings, and imports
// everything else from the module's real dependencies.
type fixtureImporter struct {
	fset     *token.FileSet
	sources  map[string]string
	packages map[string]*types.Package
	fallback types.Importer
}

func (i *fixtureImporter) Import(path string) (*types.Package, error) {
	if pkg, ok := i.packages[path]; ok {
		return pkg, nil
	}

	source, ok := i.sources[path]
	if !ok {
		return i.fallback.Import(path)
	}

	file, err := parser.ParseFile(i.fset, path+".go", source, 0)
	if err != nil {
		return nil, err
	}
	pkg, err := (&types.Config{Importer: i}).Check(path, i.fset, []*ast.File{file}, nil)
	if err != nil {
		return nil, err
	}
	i.packages[path] = pkg
	return pkg, nil
}

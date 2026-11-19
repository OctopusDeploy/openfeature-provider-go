package octopus_test

import (
	"flag"
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/OctopusDeploy/openfeature-provider-go"

var update = flag.Bool("update", false, "rewrite testdata/api.txt from the current public API")

func TestPublicApiMatchesSnapshot(t *testing.T) {
	got := apiSurface(importPublicPackage(t))
	path := filepath.Join("testdata", "api.txt")

	if *update {
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for line := range strings.Lines(string(data)) {
		if line = strings.TrimSpace(line); line != "" {
			want = append(want, line)
		}
	}

	added, removed := missingFrom(want, got), missingFrom(got, want)
	if len(added) == 0 && len(removed) == 0 {
		return
	}

	var message strings.Builder
	fmt.Fprintf(&message, "the public API doesn't match %s\n", path)
	for _, line := range added {
		fmt.Fprintf(&message, "\n+ %s", line)
	}
	for _, line := range removed {
		fmt.Fprintf(&message, "\n- %s", line)
	}
	fmt.Fprintf(&message, "\n\nIf the change is intended, run `go test -run TestPublicApi -update .` and commit %s.", path)
	t.Error(message.String())
}

func importPublicPackage(t *testing.T) *types.Package {
	t.Helper()

	pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

// apiSurface lists the package's exported API in the format of the Go
// repository's api/go1.txt, one sorted line per declaration, field and method.
// Parameter names are left out because renaming one doesn't affect callers.
func apiSurface(pkg *types.Package) []string {
	qualifier := func(other *types.Package) string {
		if other == pkg {
			return ""
		}
		return other.Name()
	}
	typeString := func(t types.Type) string { return types.TypeString(t, qualifier) }
	signatureString := func(sig *types.Signature) string {
		// Rebinding type parameters to a new signature isn't allowed, so
		// generic signatures keep their parameter names.
		if sig.TypeParams().Len() == 0 {
			sig = types.NewSignatureType(nil, nil, nil, unnamed(sig.Params()), unnamed(sig.Results()), sig.Variadic())
		}
		return strings.TrimPrefix(typeString(sig), "func")
	}

	var lines []string
	add := func(format string, args ...any) {
		lines = append(lines, "pkg "+pkg.Name()+", "+fmt.Sprintf(format, args...))
	}

	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}

		switch obj := scope.Lookup(name).(type) {
		case *types.Const:
			add("const %s %s = %s", name, typeString(obj.Type()), obj.Val().ExactString())
		case *types.Var:
			add("var %s %s", name, typeString(obj.Type()))
		case *types.Func:
			add("func %s%s", name, signatureString(obj.Signature()))
		case *types.TypeName:
			if alias, ok := obj.Type().(*types.Alias); ok {
				add("type %s = %s", name, typeString(alias.Rhs()))
				continue
			}

			named := obj.Type().(*types.Named)
			header := "type " + name + typeParamsString(named.TypeParams(), typeString)

			switch underlying := named.Underlying().(type) {
			case *types.Struct:
				add("%s struct", header)
				for field := range underlying.Fields() {
					switch {
					case !field.Exported():
					case field.Embedded():
						add("%s struct, embedded %s", header, typeString(field.Type()))
					default:
						add("%s struct, %s %s", header, field.Name(), typeString(field.Type()))
					}
				}
			case *types.Interface:
				if !underlying.IsMethodSet() {
					add("%s %s", header, typeString(underlying))
					continue
				}
				add("%s interface", header)
				for method := range underlying.Methods() {
					if method.Exported() {
						add("%s interface, %s%s", header, method.Name(), signatureString(method.Signature()))
					} else {
						add("%s interface, unexported methods", header)
					}
				}
			default:
				add("%s %s", header, typeString(underlying))
			}

			for method := range named.Methods() {
				if !method.Exported() {
					continue
				}
				receiver := name
				if _, ok := method.Signature().Recv().Type().(*types.Pointer); ok {
					receiver = "*" + name
				}
				add("method (%s) %s%s", receiver, method.Name(), signatureString(method.Signature()))
			}
		}
	}

	slices.Sort(lines)
	return slices.Compact(lines)
}

func unnamed(tuple *types.Tuple) *types.Tuple {
	vars := make([]*types.Var, tuple.Len())
	for i := range vars {
		vars[i] = types.NewParam(token.NoPos, tuple.At(i).Pkg(), "", tuple.At(i).Type())
	}
	return types.NewTuple(vars...)
}

func typeParamsString(params *types.TypeParamList, typeString func(types.Type) string) string {
	if params.Len() == 0 {
		return ""
	}
	parts := make([]string, params.Len())
	for i := range parts {
		param := params.At(i)
		parts[i] = param.Obj().Name() + " " + typeString(param.Constraint())
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// missingFrom returns the lines in b that aren't in a.
func missingFrom(a, b []string) []string {
	var missing []string
	for _, line := range b {
		if !slices.Contains(a, line) {
			missing = append(missing, line)
		}
	}
	return missing
}

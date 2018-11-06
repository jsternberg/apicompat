package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/types"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dave/jennifer/jen"
	"golang.org/x/tools/go/packages"
)

func typeOf(typ types.Type) (*jen.Statement, error) {
	switch typ := typ.(type) {
	case *types.Basic:
		return jen.Id(typ.Name()), nil
	case *types.Named:
		if !typ.Obj().Exported() {
			if typ.Obj().Name() == "error" && typ.Obj().Pkg() == nil {
				return jen.Id("error"), nil
			}
			return nil, fmt.Errorf("cannot use unexported type: %s", typ)
		}
		pkg := typ.Obj().Pkg()
		if pkg != nil {
			return jen.Qual(pkg.Path(), typ.Obj().Name()), nil
		}
		return jen.Id(typ.Obj().Name()), nil
	case *types.Pointer:
		code, err := typeOf(typ.Elem())
		if err != nil {
			return nil, err
		}
		return jen.Op("*").Add(code), nil
	case *types.Slice:
		code, err := typeOf(typ.Elem())
		if err != nil {
			return nil, err
		}
		return jen.Op("[]").Add(code), nil
	case *types.Map:
		key, err := typeOf(typ.Key())
		if err != nil {
			return nil, err
		}
		value, err := typeOf(typ.Elem())
		if err != nil {
			return nil, err
		}
		return jen.Map(key).Add(value), nil
	default:
		panic("unimplemented")
	}
}

func generateFunctionDecl(pkg *packages.Package, decl *ast.FuncDecl) (string, jen.Code, error) {
	if decl.Recv != nil || !decl.Name.IsExported() {
		return "", nil, nil
	}

	stmt, err := func() (*jen.Statement, error) {
		stmt := jen.Var().Id(decl.Name.Name).Func()
		if params, err := func() (params []jen.Code, err error) {
			for _, param := range decl.Type.Params.List {
				n := len(param.Names)
				if n == 0 {
					n = 1
				}

				for i := 0; i < n; i++ {
					paramType := param.Type
					if ellipsis, ok := paramType.(*ast.Ellipsis); ok {
						typ, err := typeOf(pkg.TypesInfo.TypeOf(ellipsis.Elt))
						if err != nil {
							return nil, err
						}
						params = append(params, jen.Op("...").Add(typ))
						continue
					}

					typ, err := typeOf(pkg.TypesInfo.TypeOf(param.Type))
					if err != nil {
						return nil, err
					}
					params = append(params, typ)
				}
			}
			return params, nil
		}(); err != nil {
			return nil, err
		} else {
			stmt = stmt.Params(params...)
		}

		if n := decl.Type.Results.NumFields(); n == 1 {
			param := decl.Type.Results.List[0]
			typ, err := typeOf(pkg.TypesInfo.TypeOf(param.Type))
			if err != nil {
				return nil, err
			}
			stmt = stmt.Add(typ)
		} else if n >= 2 {
			if params, err := func() (params []jen.Code, err error) {
				for _, param := range decl.Type.Results.List {
					n := len(param.Names)
					if n == 0 {
						n = 1
					}

					for i := 0; i < n; i++ {
						typ, err := typeOf(pkg.TypesInfo.TypeOf(param.Type))
						if err != nil {
							return nil, err
						}
						params = append(params, typ)
					}
				}
				return params, nil
			}(); err != nil {
				return nil, err
			} else {
				stmt = stmt.Params(params...)
			}
		}
		return stmt, nil
	}()
	if err != nil {
		return "", nil, err
	}
	stmt = stmt.Op("=").Qual(pkg.PkgPath, decl.Name.Name)
	return decl.Name.Name, stmt, err
}

func modulePath() (string, error) {
	cmd := exec.Command("go", "list", "-m")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", err
	}
	path, err := ioutil.ReadAll(out)
	if err != nil {
		return "", err
	}
	out.Close()

	if err := cmd.Wait(); err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(path)), nil
}

func process(module string, pkg *packages.Package) error {
	if !strings.HasPrefix(pkg.PkgPath, module) {
		return nil
	}

	fmt.Println(pkg.PkgPath)
	snippets := make(map[string]jen.Code)
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if name, code, err := generateFunctionDecl(pkg, decl); err != nil {
					// todo(jsternberg): handle errors.
					panic(err)
				} else if code != nil {
					snippets[name] = code
				}
			case *ast.GenDecl:
			}
		}
	}

	names := make([]string, 0, len(snippets))
	for name := range snippets {
		names = append(names, name)
	}
	sort.Strings(names)

	apicompat := jen.NewFile(pkg.Name)
	for _, name := range names {
		apicompat.Add(snippets[name])
	}

	suffix := strings.TrimLeft(strings.TrimPrefix(pkg.PkgPath, module), "/")
	pkgdir := filepath.Join("internal/apicompat", suffix)
	if err := os.MkdirAll(pkgdir, 0777); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(pkgdir, "apicompat.go"))
	if err != nil {
		return err
	}
	f.WriteString(apicompat.GoString())
	return f.Close()
}

func main() {
	module, err := modulePath()
	if err != nil {
		panic(err)
	}

	// todo: this should read all files regardless of platform.
	cfg := packages.Config{
		Mode: packages.LoadSyntax,
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"./..."}
	}
	pkgs, err := packages.Load(&cfg, args...)
	if err != nil {
		panic(err)
	}

	for _, pkg := range pkgs {
		if err := process(module, pkg); err != nil {
			panic(err)
		}
	}
}

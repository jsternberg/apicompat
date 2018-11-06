package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"sort"

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

func main() {
	// todo: this should read all files regardless of platform.
	cfg := packages.Config{
		Mode: packages.LoadSyntax,
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"."}
	}
	pkgs, err := packages.Load(&cfg, args...)
	if err != nil {
		panic(err)
	}

	snippets := make(map[string]jen.Code)
	for _, pkg := range pkgs {
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
					//for _, spec := range decl.Specs {
					//	switch spec := spec.(type) {
					//	case *ast.TypeSpec:
					//		if !spec.Name.IsExported() {
					//			continue
					//		}
					//
					//		switch typ := spec.Type.(type) {
					//		case *ast.InterfaceType:
					//			// Define a struct with the same name.
					//			main.Type().Id(spec.Name.Name).Struct()
					//			main.Var().Id("_").Qual(
					//				pkg.PkgPath, spec.Name.Name,
					//			).Op("=").Id(spec.Name.Name).Values()
					//
					//			// Define functions for each of the interface methods.
					//			for _, method := range typ.Methods.List {
					//				switch typ := method.Type.(type) {
					//				case *ast.FuncType:
					//					for _, name := range method.Names {
					//						if !name.IsExported() {
					//							continue
					//						}
					//						main.Func().Params(jen.Id(spec.Name.Name)).Id(name.Name)
					//					}
					//				}
					//				fmt.Printf("%v %T\n", method.Names, method.Type)
					//				//main.Func().Params(jen.Id(spec.Name.Name)).Id(
					//				//	method.
					//				//	)
					//			}
					//
					//			//main.Func().Params(jen.Id(spec.Name.Name)).
					//		}
					//	}
					//}
				}
			}
		}
	}

	names := make([]string, 0, len(snippets))
	for name := range snippets {
		names = append(names, name)
	}
	sort.Strings(names)

	main := jen.NewFile("apicompat")
	for _, name := range names {
		main.Add(snippets[name])
	}
	fmt.Println(main.GoString())
}

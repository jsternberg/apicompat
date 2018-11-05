package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"

	"github.com/dave/jennifer/jen"
	"golang.org/x/tools/go/packages"
)

func typeOf(typ types.Type) *jen.Statement {
	switch typ := typ.(type) {
	case *types.Basic:
		return jen.Id(typ.Name())
	case *types.Named:
		pkg := typ.Obj().Pkg()
		if pkg != nil {
			return jen.Qual(pkg.Path(), typ.Obj().Name())
		}
		return jen.Id(typ.Obj().Name())
	case *types.Pointer:
		return jen.Op("*").Add(typeOf(typ.Elem()))
	case *types.Slice:
		return jen.Op("[]").Add(typeOf(typ.Elem()))
	case *types.Map:
		return jen.Map(typeOf(typ.Key())).Add(typeOf(typ.Elem()))
	default:
		panic("unimplemented")
	}
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

	main := jen.NewFile("apicompat")
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			for _, decl := range f.Decls {
				switch decl := decl.(type) {
				case *ast.FuncDecl:
					if decl.Recv != nil {
						continue
					} else if !decl.Name.IsExported() {
						continue
					}

					main.Var().Id(decl.Name.Name).Func().ParamsFunc(func(g *jen.Group) {
						for _, param := range decl.Type.Params.List {
							n := len(param.Names)
							if n == 0 {
								n = 1
							}

							for i := 0; i < n; i++ {
								paramType := param.Type
								if ellipsis, ok := paramType.(*ast.Ellipsis); ok {
									g.Op("...").Add(typeOf(pkg.TypesInfo.TypeOf(ellipsis.Elt)))
									continue
								}
								typ := typeOf(pkg.TypesInfo.TypeOf(param.Type))
								g.Add(typ)
							}
						}
					}).Do(func(stmt *jen.Statement) {
						if n := decl.Type.Results.NumFields(); n == 0 {
							return
						} else if n == 1 {
							param := decl.Type.Results.List[0]
							stmt.Add(typeOf(pkg.TypesInfo.TypeOf(param.Type)))
						} else {
							stmt.ParamsFunc(func(g *jen.Group) {
								for _, param := range decl.Type.Results.List {
									n := len(param.Names)
									if n == 0 {
										n = 1
									}

									for i := 0; i < n; i++ {
										typ := typeOf(pkg.TypesInfo.TypeOf(param.Type))
										g.Add(typ)
									}
								}
							})
						}
					}).Op("=").Qual(pkg.PkgPath, decl.Name.Name)
				default:
					fmt.Printf("unimplemented: %T\n", decl)
				}
			}
		}
	}
	fmt.Println(main.GoString())
}

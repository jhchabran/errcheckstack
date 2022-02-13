package errcheckstack

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "errcheckstack",
		Doc:  "Checks that errors are wrapped before reaching main functions",
		Run:  run(),
	}
}

func run() func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		for _, file := range pass.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}
				if len(ret.Results) < 1 {
					return true
				}

				// Iterate over the values to be returned looking for errors
				for _, expr := range ret.Results {
					// Check if the return expression is a function call, if it is, we need
					// to handle it by checking the return params of the function.
					retFn, ok := expr.(*ast.CallExpr)
					if ok {
						// If the return type of the function is a single error. This will not
						// match an error within multiple return values, for that, the below
						// tuple check is required.
						if isError(pass.TypesInfo.TypeOf(expr)) {
							// check if wrapped
							reportUnwrapped(pass, retFn, retFn.Pos())
							fmt.Println(retFn)
							return true
						}

						// Check if one of the return values from the function is an error
						tup, ok := pass.TypesInfo.TypeOf(expr).(*types.Tuple)
						if !ok {
							return true
						}

						// Iterate over the return values of the function looking for error
						// types
						for i := 0; i < tup.Len(); i++ {
							v := tup.At(i)
							if v == nil {
								return true
							}
							if isError(v.Type()) {
								// check if wrapped
								reportUnwrapped(pass, retFn, retFn.Pos())
								return true
							}
						}
					}

					if !isError(pass.TypesInfo.TypeOf(expr)) {
						continue
					}

					ident, ok := expr.(*ast.Ident)
					if !ok {
						return true
					}

					// Attempt to find the most recent short assign
					var call *ast.CallExpr
					if shortAss := prevErrAssign(pass, file, ident); shortAss != nil {
						call, ok = shortAss.Rhs[0].(*ast.CallExpr)
						if !ok {
							return true
						}
					} else if isUnresolved(file, ident) {
						// TODO Check if the identifier is unresolved, and try to resolve it in
						// another file.
						fmt.Println(file)
						return true
					} else {
						// Check for ValueSpec nodes in order to locate a possible var
						// declaration.
						if ident.Obj == nil {
							return true
						}

						vSpec, ok := ident.Obj.Decl.(*ast.ValueSpec)
						if !ok {
							// We couldn't find a short or var assign for this error return.
							// This is an error. Where did this identifier come from? Possibly a
							// function param.
							//
							// TODO decide how to handle this case, whether to follow function
							// param back, or assert wrapping at call site.
							return true
						}

						if len(vSpec.Values) < 1 {
							return true
						}

						call, ok = vSpec.Values[0].(*ast.CallExpr)
						if !ok {
							return true
						}
					}

					// make sure there is a call identified as producing the error being
					// returned, otherwise just bail
					if call == nil {
						return true
					}

					reportUnwrapped(pass, call, ident.NamePos)
					fmt.Println(call)
				}

				return true
			})
		}
		return nil, nil
	}
}

// isError returns whether or not the provided type interface is an error
func isError(typ types.Type) bool {
	if typ == nil {
		return false
	}

	return typ.String() == "error"
}

// Report unwrapped takes a call expression and an identifier and reports
// if the call is unwrapped.
func reportUnwrapped(pass *analysis.Pass, call *ast.CallExpr, tokenPos token.Pos) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	// Check for ignored signatures
	fnSig := pass.TypesInfo.ObjectOf(sel.Sel).String()
	fmt.Println(fnSig)
	// if contains(cfg.IgnoreSigs, fnSig) {
	// 	return
	// } else if containsMatch(cfg.IgnoreSigRegexps, fnSig) {
	// 	return
	// }

	// Check if the underlying type of the "x" in x.y.z is an interface, as
	// errors returned from interface types should be wrapped.
	if isInterface(pass, sel) {
		pass.Reportf(tokenPos, "error returned from interface method should be wrapped: sig: %s", fnSig)
		return
	}

	// Check whether the function being called comes from another package,
	// as functions called across package boundaries which returns errors
	// should be wrapped
	if isFromOtherPkg(pass, sel) {
		pass.Reportf(tokenPos, "error returned from external package is unwrapped: sig: %s", fnSig)
		return
	}
}

// isInterface returns whether the function call is one defined on an interface.
func isInterface(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	_, ok := pass.TypesInfo.TypeOf(sel.X).Underlying().(*types.Interface)

	return ok
}

func isFromOtherPkg(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	// The package of the function that we are calling which returns the error
	fn := pass.TypesInfo.ObjectOf(sel.Sel)

	fmt.Println(fn.Pkg().Path())

	// if strings.HasPrefix(fn.Pkg().Path(), "errchecktest") {
	// 	return false
	// }

	// for _, globString := range config.IgnorePackageGlobs {
	// 	g, err := glob.Compile(globString)
	// 	if err != nil {
	// 		log.Printf("unable to parse glob: %s\n", globString)
	// 		os.Exit(1)
	// 	}
	//
	// 	if g.Match(fn.Pkg().Path()) {
	// 		return false
	// 	}
	// }

	// If it's not a package name, then we should check the selector to make sure
	// that it's an identifier from the same package
	if pass.Pkg.Path() == fn.Pkg().Path() {
		return false
	}

	return true
}

// prevErrAssign traverses the AST of a file looking for the most recent
// assignment to an error declaration which is specified by the returnIdent
// identifier.
//
// This only returns short form assignments and reassignments, e.g. `:=` and
// `=`. This does not include `var` statements. This function will return nil if
// the only declaration is a `var` (aka ValueSpec) declaration.
func prevErrAssign(pass *analysis.Pass, file *ast.File, returnIdent *ast.Ident) *ast.AssignStmt {
	// A slice containing all the assignments which contain an identifer
	// referring to the source declaration of the error. This is to catch
	// cases where err is defined once, and then reassigned multiple times
	// within the same block. In these cases, we should check the method of
	// the most recent call.
	var assigns []*ast.AssignStmt

	// Find all assignments which have the same declaration
	ast.Inspect(file, func(n ast.Node) bool {
		if ass, ok := n.(*ast.AssignStmt); ok {
			for _, expr := range ass.Lhs {
				if !isError(pass.TypesInfo.TypeOf(expr)) {
					continue
				}
				if assIdent, ok := expr.(*ast.Ident); ok {
					if assIdent.Obj == nil || returnIdent.Obj == nil {
						// If we can't find the Obj for one of the identifiers, just skip
						// it.
						return true
					} else if assIdent.Obj.Decl == returnIdent.Obj.Decl {
						assigns = append(assigns, ass)
					}
				}
			}
		}

		return true
	})

	// Iterate through the assignments, comparing the token positions to
	// find the assignment that directly precedes the return position
	var mostRecentAssign *ast.AssignStmt

	for _, ass := range assigns {
		if ass.Pos() > returnIdent.Pos() {
			break
		}
		mostRecentAssign = ass
	}

	return mostRecentAssign
}

func contains(slice []string, el string) bool {
	for _, s := range slice {
		if strings.Contains(el, s) {
			return true
		}
	}

	return false
}

func isUnresolved(file *ast.File, ident *ast.Ident) bool {
	for _, unresolvedIdent := range file.Unresolved {
		if unresolvedIdent.Pos() == ident.Pos() {
			return true
		}
	}

	return false
}

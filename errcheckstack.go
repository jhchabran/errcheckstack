package errcheckstack

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
)

func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:       "errcheckstack",
		Doc:        "Checks that errors are wrapped before reaching main functions",
		Run:        run(),
		ResultType: reflect.TypeOf(&Result{}),
	}
}

type Result struct {
	funcs map[*types.Func]*wrappedCall
}

func run() func(pass *analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		res := &Result{
			funcs: make(map[*types.Func]*wrappedCall),
		}
		if _, err := scanErrorReturningFunctions(pass, res); err != nil {
			return nil, err
		}
		if _, err := scanNonRecursivelyWrappedFunctions(pass, res); err != nil {
			return nil, err
		}

		return res, nil
	}
}

func printStack(stack []*wrappedCall) {
	if len(stack) < 1 {
		return
	}
	for _, wc := range stack {
		fmt.Println(wc.String())
	}
}

type errorSource struct {
	n       ast.Node
	fn      *types.Func
	wrapped bool
}

func (es *errorSource) String() string {
	return fmt.Sprintf("%v -> %#v (%#v) ", es.wrapped, es.fn.String(), es.n)
}

type wrappedCall struct {
	fdecl      *ast.FuncDecl
	errSources []*errorSource
}

func (wc *wrappedCall) String() string {
	var sb strings.Builder
	sb.WriteString(wc.fdecl.Name.Name)
	sb.WriteString("\n")

	for _, es := range wc.errSources {
		sb.WriteString(fmt.Sprintf("\t%s\n", es.String()))
	}
	return sb.String()
}

func (wc *wrappedCall) IsWrapped() bool {
	for _, es := range wc.errSources {
		if !es.wrapped {
			return false
		}
	}
	return true
}

// scanErrorReturningFunctions scans the entire package to find functions that return errors
// and put them in two groups: those who are wrapping their errors and those who don't.
//
// Functions from external packages are always considered to be unwrapped.
func scanErrorReturningFunctions(pass *analysis.Pass, res *Result) (interface{}, error) {
	var stack []*wrappedCall

	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			// printStack(stack)

			// Looking at a function declaration, take note and add it to the stack
			if fdecl, ok := n.(*ast.FuncDecl); ok {
				if fdecl.Type.Results == nil {
					return true
				}

				if fdecl.Type.Results == nil {
					// That function does not return any error, skip it.
					return true
				}

				for _, r := range fdecl.Type.Results.List {
					// TODO check for function returning functions returning errors
					if pass.TypesInfo.TypeOf(r.Type).String() == "error" {
						// The function returns an error, it's a candidate for a check and
						// we add it to the stack.
						stack = append(stack, &wrappedCall{fdecl: fdecl})
						return true
					}
				}
			}

			var lastFdecl *wrappedCall
			if len(stack) > 0 {
				lastFdecl = stack[len(stack)-1]
			} else {
				// We are not inside a function, continue exploring the ast until we find one.
				return true
			}

			// if ident, ok := n.(*ast.Ident); ok {
			// 	if shortAss := prevErrAssign(pass, file, ident); shortAss != nil {
			// 		call, cok := shortAss.Rhs[0].(*ast.CallExpr)
			// 		if !cok {
			// 			return true
			// 		}
			// 		b := checkUnwrapped(pass, call, ident.Pos())
			// 		lastFdecl.errSources = append(lastFdecl.errSources, &errorSource{wrapped: b, n: call})
			// 	}
			// }

			// Looking at a return statement, search if it includes an error, if yes
			// check if that error is wrapped.
			if ret, ok := n.(*ast.ReturnStmt); ok {
				for _, expr := range ret.Results {
					// Check if the return expression is a function call, if it is, we need
					// to handle it by checking the return params of the function.
					retFn, rok := expr.(*ast.CallExpr)
					if rok {
						// If the return type of the function is a single error. This will not
						// match an error within multiple return values, for that, the below
						// tuple check is required.
						if isError(pass.TypesInfo.TypeOf(expr)) {
							b := checkUnwrapped(pass, retFn, retFn.Pos())
							fn := extractFunc(pass.TypesInfo, retFn.Fun)
							lastFdecl.errSources = append(lastFdecl.errSources, &errorSource{wrapped: b, n: n, fn: fn})
							return true
						}
					}

					if !isError(pass.TypesInfo.TypeOf(expr)) {
						continue
					}

					ident, iok := expr.(*ast.Ident)
					if !iok {
						return true
					}

					var call *ast.CallExpr

					// Attempt to find the most recent short assign
					assignments := prevErrAssign(pass, file, ident)
					for _, shortAss := range assignments {
						if shortAss != nil {
							call, ok = shortAss.Rhs[0].(*ast.CallExpr)
							if !ok {
								return true
							}
							b := checkUnwrapped(pass, call, ident.NamePos)
							fn := extractFunc(pass.TypesInfo, call.Fun)
							fmt.Println(lastFdecl.fdecl.Name)
							fmt.Println(fn.String())
							lastFdecl.errSources = append(lastFdecl.errSources, &errorSource{wrapped: b, n: n, fn: fn})
						} else if isUnresolved(file, ident) {
							// TODO Check if the identifier is unresolved, and try to resolve it in
							// another file.
							fmt.Println("unresolved", file)
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
					}

					// Make sure there is a call identified as producing the error being
					// returned, otherwise just bail
					if call == nil {
						return true
					}
					b := checkUnwrapped(pass, call, ident.NamePos)
					fn := extractFunc(pass.TypesInfo, call.Fun)
					lastFdecl.errSources = append(lastFdecl.errSources, &errorSource{wrapped: b, n: n, fn: fn})
				}
			}

			// fn, ok := pass.TypesInfo.Defs[fdecl.Name].(*types.Func)
			// Type information may be incomplete.

			fn, _ := pass.TypesInfo.Defs[lastFdecl.fdecl.Name].(*types.Func)
			res.funcs[fn] = lastFdecl
			return true
		})
	}

	return nil, nil
}

// scanNonRecursivelyWrappedFunctions uses the results from scanErrorReturningFunctions to
// eliminate from the results functions which have their errors wrapped in the call tree.
func scanNonRecursivelyWrappedFunctions(pass *analysis.Pass, res *Result) (interface{}, error) {
	for fn, w := range res.funcs {
		fmt.Println(fn.FullName(), w.String())
		fmt.Printf("%#v\n\n", fn)
	}

	for fn := range res.funcs {
		if !isWrappedAtSource(fn, res) {
			fmt.Println("NOK ", fn.FullName(), "is returning unwrapped error")
		} else {
			fmt.Println("OK  ", fn.FullName(), "is good")
		}
	}

	return nil, nil
}

// isError returns whether or not the provided type interface is an error
func isError(typ types.Type) bool {
	if typ == nil {
		return false
	}

	return typ.String() == "error"
}

func checkUnwrapped(pass *analysis.Pass, call *ast.CallExpr, tokenPos token.Pos) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	fnSig := pass.TypesInfo.ObjectOf(sel.Sel).String()
	if fnSig == "func github.com/cockroachdb/errors.WithStack(err error) error" {
		return true
	}

	// Check for ignored signatures
	// fnSig := pass.TypesInfo.ObjectOf(sel.Sel).String()
	// if contains(cfg.IgnoreSigs, fnSig) {
	// 	return
	// } else if containsMatch(cfg.IgnoreSigRegexps, fnSig) {
	// 	return
	// }

	// Check if the underlying type of the "x" in x.y.z is an interface, as
	// errors returned from interface types should be wrapped.
	if isInterface(pass, sel) {
		return true
	}

	// Check whether the function being called comes from another package,
	// as functions called across package boundaries which returns errors
	// should be wrapped
	if isFromOtherPkg(pass, sel) {
		return false
	}

	return false
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

func isWrappedAtSource(fn *types.Func, res *Result) bool {
	fmt.Println("inspecting", fn.FullName())
	w, ok := res.funcs[fn]
	if !ok {
		fmt.Println("did not find", fn.FullName())
		fmt.Printf("%#v\n", fn)
		return false
	}

	// Search in error sources until we find a non wrapped error, if any.
	for _, es := range w.errSources {
		if !es.wrapped {
			if _, ok := res.funcs[es.fn]; ok {
				// The source function is not from any files in the pass and not wrapped.
				return false
			} else {
				b := isWrappedAtSource(es.fn, res)
				fmt.Println("found call to", es.fn.FullName(), b)
				return b
			}
		}
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
func prevErrAssign(pass *analysis.Pass, file *ast.File, returnIdent *ast.Ident) []*ast.AssignStmt {
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
	// var mostRecentAssign *ast.AssignStmt
	//
	// for _, ass := range assigns {
	// 	if ass.Pos() > returnIdent.Pos() {
	// 		break
	// 	}
	// 	mostRecentAssign = ass
	// }

	// return mostRecentAssign
	return assigns
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

func extractFunc(typesInfo *types.Info, fun ast.Expr) *types.Func {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	fn, ok := typesInfo.ObjectOf(sel.Sel).(*types.Func)
	if ok {
		return fn
	}
	return nil
}

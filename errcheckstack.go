package errcheckstack

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Using analysis.Fact triggers the analyzer to load not only the given packages, but all
// others that are imported. We definitely do not need to do that as there is no point in
// checking those packages which aren't in the scope given to the analyzer (i.e ./...).
var Analyzer = &analysis.Analyzer{
	Name:      "errcheckstack",
	Doc:       "Checks that errors are wrapped before reaching main functions",
	Run:       run(),
	FactTypes: []analysis.Fact{new(wrapFact)},
}

var moduleName string

func init() {
	Analyzer.Flags.StringVar(&moduleName, "module", moduleName, "module to analyze")
}

type wrapFact struct {
	isWrapped bool
}

func (w wrapFact) AFact() {}

func (w wrapFact) String() string {
	if w.isWrapped {
		return "wrapped"
	} else {
		return "unwrapped"
	}
}

func run() func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		if moduleName == "" {
			return nil, fmt.Errorf("no module name given")
		}

		pkgPath := pass.Pkg.Path()
		if !strings.HasPrefix(pkgPath, moduleName) {
			// We don't care about this module, immediately return empty results
			return nil, nil
		}

		if _, err := scanErrorReturningFunctions(pass); err != nil {
			return nil, err
		}
		return nil, nil
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
func scanErrorReturningFunctions(pass *analysis.Pass) (interface{}, error) {
	var stack []*wrappedCall

	for _, file := range pass.Files {
		// Because we aren't going over the AST more than once, we don't use inspect.Inspector,
		// which provides a speed up on the fifth traversal of the AST, which is not the case
		// here as we're only going through it once.
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
							if !b {
								pass.Reportf(retFn.Pos(), "is not wrapped")
							}
							fn := extractFunc(pass.TypesInfo, retFn.Fun)
							ffn, ok := pass.TypesInfo.ObjectOf(lastFdecl.fdecl.Name).(*types.Func)
							if ok {
								pass.ExportObjectFact(ffn, &wrapFact{isWrapped: b && lastFdecl.IsWrapped()})
							}
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
							// fmt.Println(lastFdecl.fdecl.Name)
							// fmt.Println(fn.String())
							lastFdecl.errSources = append(lastFdecl.errSources, &errorSource{wrapped: b, n: n, fn: fn})
							pass.ExportObjectFact(fn, &wrapFact{isWrapped: b})
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
					pass.ExportObjectFact(fn, &wrapFact{isWrapped: b})
				}
			}

			// fn, ok := pass.TypesInfo.Defs[fdecl.Name].(*types.Func)
			// Type information may be incomplete.

			return true
		})
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
	fn := pass.TypesInfo.ObjectOf(sel.Sel).(*types.Func)
	fnSig := fn.String()
	if fnSig == "func github.com/cockroachdb/errors.WithStack(err error) error" {
		return true
	}

	fact := wrapFact{}
	pass.ImportObjectFact(fn, &fact)
	if fact.isWrapped {
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

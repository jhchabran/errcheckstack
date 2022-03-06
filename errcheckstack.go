package errcheckstack

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

type Config struct {
	// WrappingSignatures defines what function signature is considered
	// as error wrapping. So errors return by these functions will not
	// create a diagnostic.
	WrappingSignatures []string `yaml:"wrappingSignatures"`
	// In order to function, this analyzer requires to be passed a module name so it avoids
	// inspecting any other packages than the ones in that module.
	ModuleName string `yaml:"moduleName"`
}

func NewAnalyzer(cfg Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:      "errcheckstack",
		Doc:       "Checks that errors are wrapped before reaching main functions",
		Run:       run(cfg),
		FactTypes: []analysis.Fact{new(wrapFact)},
	}
}

// wrapFact represents if an object is wrapped or not.
type wrapFact struct {
	isWrapped bool
}

func (w wrapFact) AFact() {}

func (w wrapFact) String() string {
	if w.isWrapped {
		return "wrapped"
	} else {
		return "naked"
	}
}

func run(cfg Config) func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		if cfg.ModuleName == "" {
			// The analyzer cannot work without a given module to scope the search,
			// otherwise we would raise tons of diagnostics from the dependencies
			// which we do not want to raise.
			return nil, fmt.Errorf("no module name given")
		}

		// Check if the current package is to be searched or not.
		pkgPath := pass.Pkg.Path()
		if !strings.HasPrefix(pkgPath, cfg.ModuleName) {
			// We don't care about this module, immediately return empty results
			return nil, nil
		}

		return scan(&cfg, pass)
	}
}

type errorSource struct {
	fn      *types.Func
	wrapped bool
}

func (es *errorSource) String() string {
	return fmt.Sprintf("%v -> %#v", es.wrapped, es.fn.String())
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

// scan scans the entire package to find functions that return errors
// and put them in two groups: those who are wrapping their errors and those who don't.
//
// Functions from external packages are always considered to be unwrapped.
func scan(cfg *Config, pass *analysis.Pass) (interface{}, error) {
	var curFdecl *wrappedCall

	for _, file := range pass.Files {
		// Because we aren't going over the AST more than once, we don't use inspect.Inspector,
		// which provides a speed up on the fifth traversal of the AST, which is not the case
		// here as we're only going through it once.
		ast.Inspect(file, func(n ast.Node) bool {
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
						curFdecl = &wrappedCall{fdecl: fdecl}
						return true
					}
				}
			}

			if curFdecl == nil {
				// We are not inside a function, continue exploring the ast until we find one.
				return true
			}

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
							b := checkWrapped(cfg, pass, retFn, retFn.Pos())
							if !b {
								reportUnwrapped(pass, retFn, retFn.Pos())
							}
							fn := extractFunc(pass.TypesInfo, retFn.Fun)
							callerFn, ok := pass.TypesInfo.ObjectOf(curFdecl.fdecl.Name).(*types.Func)
							if ok {
								fmt.Println("wrapped: ", b && curFdecl.IsWrapped())
								pass.ExportObjectFact(callerFn, &wrapFact{isWrapped: b && curFdecl.IsWrapped()})
							}
							curFdecl.errSources = append(curFdecl.errSources, &errorSource{wrapped: b, fn: fn})
							return true
						}
					}

					// Check if that element of the return tuple is an error.
					if !isError(pass.TypesInfo.TypeOf(expr)) {
						continue
					}

					// It is an error. Let's find where it's been assigned, so we can check it.
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
							b := checkWrapped(cfg, pass, call, ident.NamePos)
							fn := extractFunc(pass.TypesInfo, call.Fun)
							curFdecl.errSources = append(curFdecl.errSources, &errorSource{wrapped: b, fn: fn})
							if !b {
								reportUnwrapped(pass, call, ident.NamePos)
							}
							sel, ok := call.Fun.(*ast.SelectorExpr)
							if ok {
								if !isFromOtherPkg(pass, sel) {
									pass.ExportObjectFact(fn, &wrapFact{isWrapped: b})
								}
							}
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
					// returned, otherwise just bail.
					if call == nil {
						return true
					}
					b := checkWrapped(cfg, pass, call, ident.NamePos)
					fn := extractFunc(pass.TypesInfo, call.Fun)
					curFdecl.errSources = append(curFdecl.errSources, &errorSource{wrapped: b, fn: fn})
					callerFn, ok := pass.TypesInfo.ObjectOf(curFdecl.fdecl.Name).(*types.Func)
					if ok {
						pass.ExportObjectFact(callerFn, &wrapFact{isWrapped: b && curFdecl.IsWrapped()})
					}
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

func checkWrapped(cfg *Config, pass *analysis.Pass, call *ast.CallExpr, tokenPos token.Pos) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check if that function call is part of the wrapping functions.
	fn := pass.TypesInfo.ObjectOf(sel.Sel).(*types.Func)
	for _, fullname := range cfg.WrappingSignatures {
		if fn.FullName() == fullname {
			return true
		}
	}

	// Check if that function call is marked as wrapped by a previous pass.
	fact := wrapFact{}
	if ok := pass.ImportObjectFact(fn, &fact); ok {
		if fact.isWrapped {
			return true
		}
	}

	// Check if the underlying type of the "x" in x.y.z is an interface, as
	// errors returned from interface types should be wrapped.
	if isInterface(pass, sel) {
		return false
	}

	// Check whether the function being called comes from another package,
	// that is not part of the analysis and therefore should be wrapped.
	if isFromOtherPkg(pass, sel) {
		return false
	}

	return false
}

// isInterface returns whether the function call is one defined on an interface.
func isInterface(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	_, ok := pass.TypesInfo.TypeOf(sel.X).Underlying().(*types.Interface)
	return ok
}

func isFromOtherPkg(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	// The package of the function that we are calling which returns the error
	fn := pass.TypesInfo.ObjectOf(sel.Sel)

	// If it's not a package name, then we should check the selector to make sure
	// that it's an identifier from the same package
	if pass.Pkg.Path() == fn.Pkg().Path() {
		return false
	}

	return true
}

// prevErrAssign traverses the AST of a file looking for the all assignments
// to an error declaration which is specified by the returnIdent identifier.
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
						// If we can't find the Obj for one of the identifiers, just skip it.
						return true
					} else if assIdent.Obj.Decl == returnIdent.Obj.Decl {
						assigns = append(assigns, ass)
					}
				}
			}
		}

		return true
	})

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

func reportUnwrapped(pass *analysis.Pass, call *ast.CallExpr, tokenPos token.Pos) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	if isInterface(pass, sel) {
		pass.Reportf(tokenPos, "error returned from interface type is not wrapped")
		return
	}

	if isFromOtherPkg(pass, sel) {
		pass.Reportf(tokenPos, "error returned from external package is not wrapped")
		return
	}

	pass.Reportf(tokenPos, "error returned is not wrapped")
}

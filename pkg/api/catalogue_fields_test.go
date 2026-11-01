package api

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// updateCatalogue rewrites catalogue_fields.json from the engine's source.
var updateCatalogue = flag.Bool("update-catalogue", false,
	"rewrite pkg/api/catalogue_fields.json from the engine's source")

// The catalogue's descriptions, spec fields and params are read from the
// engine's source here, and kept in catalogue_fields.json, which the binary
// embeds:
//   - params: the names each registered type's methods, and the same-package
//     functions they call, read with params.Get, GetIndex, GetCopy, GetOk or
//     GetCopyOk; a constant is resolved, "prefix_" + i becomes prefix_*, and
//     any other name marks the type's params incomplete;
//   - spec fields: the keys each registry's builder reads, through the strict
//     field readers or the spec's map, or decodes into a typed struct;
//   - the typed specs of macros, sources, streams and environments, by
//     reflection on their yaml tags.

// sourcePackages are the engine's packages a registered type can live in.
var sourcePackages = []string{"agents", "analysis", "api", "continuous", "discrete",
	"general", "inference", "kernels", "macros", "simulator"}

// goPackage is one package's parsed source, indexed for the scan.
type goPackage struct {
	name    string
	funcs   map[string]*ast.FuncDecl
	methods map[string]map[string]*ast.FuncDecl
	consts  map[string]string
	structs map[string]*ast.StructType
	docs    map[string]string
	vars    map[string]ast.Expr
	files   []*ast.File
}

// goSource is every package in sourcePackages, by name.
type goSource map[string]*goPackage

func parseSource(t *testing.T) goSource {
	t.Helper()
	source := goSource{}
	for _, name := range sourcePackages {
		dir := filepath.Join(repoDir, "pkg", name)
		fileSet := token.NewFileSet()
		packages, err := parser.ParseDir(fileSet, dir, func(info os.FileInfo) bool {
			return !strings.HasSuffix(info.Name(), "_test.go")
		}, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		pkg := &goPackage{name: name, funcs: map[string]*ast.FuncDecl{},
			methods: map[string]map[string]*ast.FuncDecl{}, consts: map[string]string{},
			structs: map[string]*ast.StructType{}, docs: map[string]string{}, vars: map[string]ast.Expr{}}
		for _, parsed := range packages {
			for _, file := range parsed.Files {
				pkg.files = append(pkg.files, file)
				indexFile(pkg, file)
			}
		}
		source[name] = pkg
	}
	return source
}

func indexFile(pkg *goPackage, file *ast.File) {
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			if typed.Recv == nil {
				pkg.funcs[typed.Name.Name] = typed
				continue
			}
			receiver := receiverType(typed.Recv.List[0].Type)
			if pkg.methods[receiver] == nil {
				pkg.methods[receiver] = map[string]*ast.FuncDecl{}
			}
			pkg.methods[receiver][typed.Name.Name] = typed
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					for index, name := range spec.Names {
						if index < len(spec.Values) {
							pkg.vars[name.Name] = spec.Values[index]
							if literal, ok := spec.Values[index].(*ast.BasicLit); ok &&
								literal.Kind == token.STRING {
								pkg.consts[name.Name], _ = strconv.Unquote(literal.Value)
							}
						}
					}
				case *ast.TypeSpec:
					if structType, ok := spec.Type.(*ast.StructType); ok {
						pkg.structs[spec.Name.Name] = structType
					}
					text := spec.Doc.Text()
					if text == "" && len(typed.Specs) == 1 {
						text = typed.Doc.Text()
					}
					pkg.docs[spec.Name.Name] = firstSentence(text)
				}
			}
		}
	}
}

// receiverType is a method's receiver type name, without pointer or type
// parameters.
func receiverType(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return receiverType(typed.X)
	case *ast.IndexExpr:
		return receiverType(typed.X)
	case *ast.IndexListExpr:
		return receiverType(typed.X)
	case *ast.Ident:
		return typed.Name
	}
	return ""
}

// firstSentence is a doc comment's first sentence, on one line.
func firstSentence(text string) string {
	var parser doc.Package
	return strings.Join(strings.Fields(parser.Synopsis(text)), " ")
}

// reads collects the params one function reads, and the functions it calls
// that may read more.
type paramReads struct {
	names      map[string]bool // name -> optional
	incomplete bool
}

// paramGetters are the Params methods a type reads a param with, and whether
// each tolerates its absence.
var paramGetters = map[string]bool{"Get": false, "GetIndex": false, "GetCopy": false,
	"GetOk": true, "GetCopyOk": true}

// typeParams is what source says a type in pkg reads from its params.
func (s goSource) typeParams(pkg *goPackage, typeName string) paramReads {
	var roots []ast.Node
	for _, method := range sortedDecls(pkg.methods[typeName]) {
		roots = append(roots, method)
	}
	return s.readParams(pkg, typeName, roots)
}

// readParams is what source says roots (functions of pkg, and methods of
// typeName when it is set) read from params, following the same-package
// functions they call or pass on, and typeName's methods they call.
func (s goSource) readParams(pkg *goPackage, typeName string, roots []ast.Node) paramReads {
	reads := paramReads{names: map[string]bool{}}
	type pending struct {
		pkg  *goPackage
		node ast.Node
	}
	var queue []pending
	seen := map[ast.Node]bool{}
	enqueue := func(p *goPackage, node ast.Node) {
		if node != nil && !reflect.ValueOf(node).IsNil() && !seen[node] {
			seen[node] = true
			queue = append(queue, pending{p, node})
		}
	}
	for _, root := range roots {
		enqueue(pkg, root)
	}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		// The variables of type *simulator.Params in scope, whatever their names.
		paramsVars := map[string]bool{}
		ast.Inspect(next.node, func(node ast.Node) bool {
			if field, ok := node.(*ast.Field); ok && isParamsType(field.Type) {
				for _, name := range field.Names {
					paramsVars[name.Name] = true
				}
			}
			return true
		})
		isParams := func(expression ast.Expr) bool {
			name := exprName(expression)
			return paramsVars[name] || strings.Contains(strings.ToLower(name), "params")
		}
		ast.Inspect(next.node, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.Ident:
				// A same-package function called, or passed on as a value.
				if decl := next.pkg.funcs[typed.Name]; decl != nil {
					enqueue(next.pkg, decl)
				}
			case *ast.SelectorExpr:
				if typed.Sel.Name == "Map" && isParams(typed.X) {
					reads.incomplete = true // params read by names known only at run time
				}
				if typeName != "" {
					// A method of the type itself, called on its receiver.
					enqueue(next.pkg, pkg.methods[typeName][typed.Sel.Name])
				}
			case *ast.CallExpr:
				callee, ok := typed.Fun.(*ast.SelectorExpr)
				if !ok {
					break
				}
				optional, getter := paramGetters[callee.Sel.Name]
				if !getter || !isParams(callee.X) || len(typed.Args) == 0 {
					break
				}
				name, ok := s.paramName(next.pkg, typed.Args[0])
				if !ok {
					reads.incomplete = true
					break
				}
				was, read := reads.names[name]
				reads.names[name] = optional && (!read || was)
			}
			return true
		})
	}
	return reads
}

// isParamsType reports whether a type expression is *simulator.Params.
func isParamsType(expression ast.Expr) bool {
	star, ok := expression.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch typed := star.X.(type) {
	case *ast.SelectorExpr:
		return typed.Sel.Name == "Params" && isIdent(typed.X, "simulator")
	case *ast.Ident:
		return typed.Name == "Params"
	}
	return false
}

// exprName is the last name in an expression: params in params, Params in
// settings.Iterations[i].Params.
func exprName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	case *ast.IndexExpr:
		return exprName(typed.X)
	}
	return ""
}

// paramName resolves a param name argument: a literal, a constant, or a
// literal prefix and a run-time suffix, as prefix*.
func (s goSource) paramName(pkg *goPackage, argument ast.Expr) (string, bool) {
	switch typed := argument.(type) {
	case *ast.BasicLit:
		if typed.Kind == token.STRING {
			value, err := strconv.Unquote(typed.Value)
			return value, err == nil
		}
	case *ast.Ident:
		value, ok := pkg.consts[typed.Name]
		return value, ok
	case *ast.SelectorExpr:
		if packageName, ok := typed.X.(*ast.Ident); ok && s[packageName.Name] != nil {
			value, ok := s[packageName.Name].consts[typed.Sel.Name]
			return value, ok
		}
	case *ast.BinaryExpr:
		if typed.Op == token.ADD {
			if prefix, ok := s.paramName(pkg, typed.X); ok && !strings.HasSuffix(prefix, "*") {
				return prefix + "*", true
			}
		}
	}
	return "", false
}

func sortedDecls(decls map[string]*ast.FuncDecl) []*ast.FuncDecl {
	names := keys(decls)
	sort.Strings(names)
	sorted := make([]*ast.FuncDecl, len(names))
	for index, name := range names {
		sorted[index] = decls[name]
	}
	return sorted
}

// specField is one key a builder reads, and the registry of named values it
// takes one of, if it does.
type specField struct {
	required bool
	registry string
}

// readerRequired is whether each reader method fails on a missing key: the
// strict field readers of pkg/api (specReader) and pkg/simulator
// (fieldReader). The generic helpers that take a reader read optionally.
var readerRequired = map[string]bool{
	// specReader
	"value": false, "nestedSpec": false, "optBool": false, "intField": true,
	"floatField": true, "kernel": true, "likelihood": true, "jump": true,
	"iteration": true, "priorList": true, "iterationByEvent": true, "environment": true,
	// fieldReader
	"float": true, "int": true, "uint64": true, "str": true, "stringSlice": true,
}

// builderFields is what source says a registry builder's spec takes: the keys
// expression reads, following the same-package functions it calls a level or
// two deep.
func (s goSource) builderFields(pkg *goPackage, expression ast.Node) map[string]specField {
	fields := map[string]specField{}
	add := func(key string, required bool, registry string) {
		field := fields[key]
		field.required = field.required || required
		if registry != "" {
			field.registry = registry
		}
		fields[key] = field
	}
	seen := map[*ast.FuncDecl]bool{}
	var scan func(root ast.Node, depth int)
	scan = func(root ast.Node, depth int) {
		ast.Inspect(root, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.CallExpr:
				s.callFields(pkg, typed, root, add)
				if callee, ok := typed.Fun.(*ast.Ident); ok && depth < 2 {
					if decl := pkg.funcs[callee.Name]; decl != nil && !seen[decl] && callee.Name != "nullary" {
						seen[decl] = true
						scan(decl, depth+1)
					}
				}
			case *ast.IndexExpr:
				// fields["partitions"], spec.Fields["input"]
				if name := strings.ToLower(exprName(typed.X)); name == "fields" || name == "f" {
					if key, ok := stringLiteral(typed.Index); ok {
						add(key, false, "")
					}
				}
			case *ast.RangeStmt:
				// for field, target := range map[string]*string{"user": ..., ...}
				if literal, ok := typed.X.(*ast.CompositeLit); ok {
					for _, element := range literal.Elts {
						if entry, ok := element.(*ast.KeyValueExpr); ok {
							if key, ok := stringLiteral(entry.Key); ok {
								add(key, false, "")
							}
						}
					}
				}
			case *ast.SwitchStmt:
				// for key, value := range fields { switch key { case "input": ...
				if isIdent(typed.Tag, "key") {
					for _, clause := range typed.Body.List {
						for _, value := range clause.(*ast.CaseClause).List {
							if key, ok := stringLiteral(value); ok {
								add(key, false, "")
							}
						}
					}
				}
			case *ast.BinaryExpr:
				// for key := range fields { if key != "input" ...
				if (typed.Op == token.NEQ || typed.Op == token.EQL) && exprName(typed.X) == "key" {
					if key, ok := stringLiteral(typed.Y); ok {
						add(key, false, "")
					}
				}
			}
			return true
		})
	}
	if name, ok := expression.(*ast.Ident); ok && pkg.funcs[name.Name] != nil {
		// A builder registered by name: buildFromInput.
		expression = pkg.funcs[name.Name]
	}
	scan(expression, 0)
	return fields
}

// callFields records the keys one call reads: through a reader method, a
// generic helper given a key, or a strict decode into a typed struct.
func (s goSource) callFields(pkg *goPackage, call *ast.CallExpr, within ast.Node,
	add func(string, bool, string)) {
	switch callee := call.Fun.(type) {
	case *ast.SelectorExpr:
		if required, reader := readerRequired[callee.Sel.Name]; reader && len(call.Args) > 0 {
			if key, ok := stringLiteral(call.Args[0]); ok {
				if len(call.Args) > 1 {
					if flag, ok := call.Args[1].(*ast.Ident); ok && (flag.Name == "true" || flag.Name == "false") {
						required = flag.Name == "true"
					}
				}
				add(key, required, "")
				return
			}
		}
		if callee.Sel.Name == "UnmarshalStrict" && len(call.Args) == 2 {
			for _, key := range s.decodedKeys(pkg, call.Args[1], within) {
				add(key, false, "")
			}
		}
	}
	// A helper given the reader or spec and a key, and maybe whether it is
	// required: optional[int](r, "max_tree_depth", ...), namedFunc(r,
	// "function", ...), specString(spec, "table", true).
	if len(call.Args) > 1 {
		if holder, ok := call.Args[0].(*ast.Ident); ok && specHolders[holder.Name] {
			if key, ok := stringLiteral(call.Args[1]); ok {
				required := isIdent(call.Fun, "namedFunc")
				registry := ""
				if len(call.Args) > 2 {
					required = required || isIdent(call.Args[2], "true")
					if named, ok := call.Args[2].(*ast.Ident); ok {
						if _, isMap := pkg.vars[named.Name].(*ast.CompositeLit); isMap {
							registry = named.Name
						}
					}
				}
				add(key, required, registry)
			}
		}
	}
}

// specHolders are the names a builder gives its reader or spec.
var specHolders = map[string]bool{"r": true, "reader": true, "spec": true}

func isIdent(expression ast.Expr, name string) bool {
	ident, ok := expression.(*ast.Ident)
	return ok && ident.Name == name
}

func stringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

// decodedKeys are the top-level yaml keys of the struct a strict decode
// writes into: &spec, where spec is declared var spec T, or a variable
// assigned &pkg.T{}.
func (s goSource) decodedKeys(pkg *goPackage, target ast.Expr, within ast.Node) []string {
	if unary, ok := target.(*ast.UnaryExpr); ok {
		target = unary.X
	}
	ident, ok := target.(*ast.Ident)
	if !ok {
		return nil
	}
	var typeExpr ast.Expr
	ast.Inspect(within, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.ValueSpec:
			for _, name := range typed.Names {
				if name.Name == ident.Name && typed.Type != nil {
					typeExpr = typed.Type
				}
			}
		case *ast.AssignStmt:
			for index, left := range typed.Lhs {
				if isIdent(left, ident.Name) && index < len(typed.Rhs) {
					value := typed.Rhs[index]
					if unary, ok := value.(*ast.UnaryExpr); ok {
						value = unary.X
					}
					if literal, ok := value.(*ast.CompositeLit); ok {
						typeExpr = literal.Type
					}
				}
			}
		}
		return true
	})
	owner, structType := s.structOf(pkg, typeExpr)
	if structType == nil {
		return nil
	}
	return s.yamlTagKeys(owner, structType)
}

// structOf resolves a type expression to its struct, and the package it is in.
func (s goSource) structOf(pkg *goPackage, expression ast.Expr) (*goPackage, *ast.StructType) {
	switch typed := expression.(type) {
	case *ast.Ident:
		return pkg, pkg.structs[typed.Name]
	case *ast.SelectorExpr:
		if packageName, ok := typed.X.(*ast.Ident); ok && s[packageName.Name] != nil {
			owner := s[packageName.Name]
			return owner, owner.structs[typed.Sel.Name]
		}
	case *ast.StarExpr:
		return s.structOf(pkg, typed.X)
	}
	return pkg, nil
}

// yamlTagKeys are a struct's yaml keys, with an inline field's own keys.
func (s goSource) yamlTagKeys(pkg *goPackage, structType *ast.StructType) []string {
	var names []string
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag, _ := strconv.Unquote(field.Tag.Value)
		name, options, _ := strings.Cut(reflect.StructTag(tag).Get("yaml"), ",")
		switch {
		case strings.Contains(options, "inline"):
			if owner, inner := s.structOf(pkg, field.Type); inner != nil {
				names = append(names, s.yamlTagKeys(owner, inner)...)
			}
		case name != "" && name != "-":
			names = append(names, name)
		}
	}
	return names
}

// registryEntries finds each registry's builders in pkg: the entries of a
// map literal assigned to one of registries, an index assignment into one,
// and simulator.RegisterComponent(family, name, builder).
func registryEntries(pkg *goPackage, registries map[string]string) map[string]map[string]ast.Node {
	found := map[string]map[string]ast.Node{}
	record := func(family, name string, builder ast.Node) {
		if found[family] == nil {
			found[family] = map[string]ast.Node{}
		}
		found[family][name] = builder
	}
	literalEntries := func(family string, value ast.Expr) {
		literal, ok := value.(*ast.CompositeLit)
		if !ok {
			return
		}
		for _, element := range literal.Elts {
			if entry, ok := element.(*ast.KeyValueExpr); ok {
				if name, ok := stringLiteral(entry.Key); ok {
					record(family, name, entry.Value)
				}
			}
		}
	}
	for _, file := range pkg.files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.ValueSpec:
				for index, name := range typed.Names {
					if family, ok := registries[name.Name]; ok && index < len(typed.Values) {
						literalEntries(family, typed.Values[index])
					}
				}
			case *ast.AssignStmt:
				for index, left := range typed.Lhs {
					if index >= len(typed.Rhs) {
						continue
					}
					switch target := left.(type) {
					case *ast.Ident:
						if family, ok := registries[target.Name]; ok {
							literalEntries(family, typed.Rhs[index])
						}
					case *ast.IndexExpr:
						if family, ok := registries[exprName(target.X)]; ok {
							if name, ok := stringLiteral(target.Index); ok {
								record(family, name, typed.Rhs[index])
							}
						}
					}
				}
			case *ast.CallExpr:
				if selector, ok := typed.Fun.(*ast.SelectorExpr); ok &&
					selector.Sel.Name == "RegisterComponent" && len(typed.Args) == 3 {
					family, familyOK := stringLiteral(typed.Args[0])
					name, nameOK := stringLiteral(typed.Args[1])
					if familyOK && nameOK {
						record(catalogueFamily[family], name, typed.Args[2])
					}
				}
			}
			return true
		})
	}
	return found
}

// catalogueFamily is a component family's key in the catalogue.
var catalogueFamily = map[string]string{
	"output_function":       "output_functions",
	"output_condition":      "output_conditions",
	"termination_condition": "termination_conditions",
	"timestep_function":     "timestep_functions",
	"execution_strategy":    "execution_strategies",
}

// builtType is the type a builder constructs, as its package and name: the
// first &pkg.T{} it returns, or the T of a NewT it calls.
func (s goSource) builtType(pkg *goPackage, builder ast.Node) (*goPackage, string) {
	var owner *goPackage
	var name string
	ast.Inspect(builder, func(node ast.Node) bool {
		if name != "" {
			return false
		}
		switch typed := node.(type) {
		case *ast.UnaryExpr:
			if literal, ok := typed.X.(*ast.CompositeLit); ok && typed.Op == token.AND {
				owner, name = s.namedType(pkg, literal.Type)
			}
		case *ast.CallExpr:
			callee, calleePkg := "", pkg
			switch fun := typed.Fun.(type) {
			case *ast.Ident:
				callee = fun.Name
			case *ast.SelectorExpr:
				if packageName, ok := fun.X.(*ast.Ident); ok && s[packageName.Name] != nil {
					callee, calleePkg = fun.Sel.Name, s[packageName.Name]
				}
			}
			if typeName, ok := strings.CutPrefix(callee, "New"); ok && calleePkg.structs[typeName] != nil {
				owner, name = calleePkg, typeName
			}
		}
		return true
	})
	return owner, name
}

func (s goSource) namedType(pkg *goPackage, expression ast.Expr) (*goPackage, string) {
	switch typed := expression.(type) {
	case *ast.Ident:
		if pkg.structs[typed.Name] != nil {
			return pkg, typed.Name
		}
	case *ast.SelectorExpr:
		if packageName, ok := typed.X.(*ast.Ident); ok && s[packageName.Name] != nil {
			return s[packageName.Name], typed.Sel.Name
		}
	case *ast.IndexExpr:
		return s.namedType(pkg, typed.X)
	case *ast.IndexListExpr:
		return s.namedType(pkg, typed.X)
	}
	return nil, ""
}

// reflectedFields are a typed spec's yaml keys, with each nested mapping's
// own: a struct, or a pointer to or list of one. A {type: ...} spec or a map
// is a leaf.
func reflectedFields(t reflect.Type) []CatalogueField {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == reflect.TypeOf(simulator.ComponentSpec{}) {
		return nil
	}
	var fields []CatalogueField
	for index := 0; index < t.NumField(); index++ {
		field := t.Field(index)
		name, options, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if strings.Contains(options, "inline") {
			fields = append(fields, reflectedFields(field.Type)...)
			continue
		}
		if name == "" || name == "-" || !field.IsExported() {
			continue
		}
		fields = append(fields, CatalogueField{Name: name, Fields: reflectedFields(field.Type)})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	return fields
}

// describedTypes is what source says of each registered type, by family and
// name.
func describedTypes(t *testing.T) map[string]map[string]CatalogueEntry {
	t.Helper()
	source := parseSource(t)
	described := map[string]map[string]CatalogueEntry{}
	set := func(family, name string, entry CatalogueEntry) {
		if described[family] == nil {
			described[family] = map[string]CatalogueEntry{}
		}
		entry.Name = name
		described[family][name] = entry
	}
	withParams := func(entry CatalogueEntry, pkg *goPackage, typeName string) CatalogueEntry {
		if pkg == nil {
			return entry
		}
		entry.Description = pkg.docs[typeName]
		reads := source.typeParams(pkg, typeName)
		for _, name := range sortedKeys(reads.names) {
			entry.Params = append(entry.Params, CatalogueField{Name: name, Optional: reads.names[name]})
		}
		entry.ParamsIncomplete = reads.incomplete
		return entry
	}
	specFields := func(pkg *goPackage, builder ast.Node) []CatalogueField {
		var fields []CatalogueField
		read := source.builderFields(pkg, builder)
		for _, name := range sortedKeys(read) {
			fields = append(fields, CatalogueField{Name: name, Required: read[name].required,
				Choices: source.choices(pkg, read[name].registry)})
		}
		return fields
	}

	api := source["api"]
	builders := registryEntries(api, map[string]string{
		"iterationBuilders": "iterations", "scopedIterationBuilders": "iterations",
		"kernelBuilders": "kernels", "likelihoodBuilders": "likelihoods",
		"jumpBuilders": "jumps", "priorBuilders": "priors",
	})
	for family, entries := range registryEntries(source["simulator"], map[string]string{
		"outputConditionBuilders": "output_conditions", "outputFunctionBuilders": "output_functions",
		"terminationConditionBuilders": "termination_conditions",
		"timestepFunctionBuilders":     "timestep_functions", "executionStrategyBuilders": "execution_strategies",
	}) {
		for name, builder := range entries {
			pkg, typeName := source.builtType(source["simulator"], builder)
			entry := CatalogueEntry{Fields: specFields(source["simulator"], builder)}
			if pkg != nil {
				entry.Description = pkg.docs[typeName]
			}
			set(family, name, entry)
		}
	}
	for family, entries := range builders {
		for name, builder := range entries {
			entry := CatalogueEntry{Fields: specFields(api, builder)}
			var pkg *goPackage
			var typeName string
			if family == "iterations" {
				// The type each name constructs, which the registry drift test
				// checks.
				want := strings.TrimPrefix(wantIterationType[name], "*")
				want, _, _ = strings.Cut(want, "[")
				packageName, typeIdentifier, _ := strings.Cut(want, ".")
				pkg, typeName = source[packageName], typeIdentifier
			} else {
				pkg, typeName = source.builtType(api, builder)
			}
			if family == "iterations" || family == "kernels" || family == "likelihoods" ||
				family == "jumps" || family == "priors" {
				entry = withParams(entry, pkg, typeName)
			} else if pkg != nil {
				entry.Description = pkg.docs[typeName]
			}
			set(family, name, entry)
		}
	}
	for name, factory := range macroSpecFactories {
		set("macros", name, CatalogueEntry{Fields: withoutType(reflectedFields(reflect.TypeOf(factory())))})
	}
	for _, field := range reflectedFields(reflect.TypeOf(DataSource{})) {
		set("data_sources", field.Name, CatalogueEntry{Fields: field.Fields})
	}
	for _, field := range reflectedFields(reflect.TypeOf(StreamConfig{})) {
		set("stream_transports", field.Name, CatalogueEntry{Fields: field.Fields})
	}
	set("environments", simulationEnvironmentType, CatalogueEntry{
		Description: "A decision problem over a forward model: an action is a params " +
			"injection, a transition one step of the model, the reward another partition.",
		Fields: reflectedFields(reflect.TypeOf(simulationEnvironmentSpec{}))})
	return described
}

// choices are the named values a registry map offers: for a function, its
// description and the params it reads.
func (s goSource) choices(pkg *goPackage, registry string) []CatalogueEntry {
	literal, ok := pkg.vars[registry].(*ast.CompositeLit)
	if registry == "" || !ok {
		return nil
	}
	var choices []CatalogueEntry
	for _, element := range literal.Elts {
		entry, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := stringLiteral(entry.Key)
		if !ok {
			continue
		}
		choice := CatalogueEntry{Name: name}
		owner, function := pkg, ""
		switch value := entry.Value.(type) {
		case *ast.Ident:
			function = value.Name
		case *ast.SelectorExpr:
			if packageName, ok := value.X.(*ast.Ident); ok && s[packageName.Name] != nil {
				owner, function = s[packageName.Name], value.Sel.Name
			}
		}
		if decl := owner.funcs[function]; decl != nil {
			choice.Description = firstSentence(decl.Doc.Text())
			reads := s.readParams(owner, "", []ast.Node{decl})
			for _, param := range sortedKeys(reads.names) {
				choice.Params = append(choice.Params, CatalogueField{Name: param, Optional: reads.names[param]})
			}
			choice.ParamsIncomplete = reads.incomplete
		}
		choices = append(choices, choice)
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].Name < choices[j].Name })
	return choices
}

// withoutType drops the type discriminator a typed spec decodes.
func withoutType(fields []CatalogueField) []CatalogueField {
	kept := fields[:0]
	for _, field := range fields {
		if field.Name != "type" {
			kept = append(kept, field)
		}
	}
	return kept
}

func TestCatalogueFieldsUpToDate(t *testing.T) {
	written, err := json.MarshalIndent(describedTypes(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	written = append(written, '\n')
	path := filepath.Join(repoDir, "pkg", "api", "catalogue_fields.json")
	if *updateCatalogue {
		if err := os.WriteFile(path, written, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(written) != string(catalogueFieldsJSON) {
		t.Errorf("%s is stale; rerun with -update-catalogue", path)
	}
}

// usageFiles are the configs the usage check reads: every shipped config,
// every declarative model, and pkg/api's own config fixtures.
func usageFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, pattern := range []string{"cfg/*.yaml", "models/*/declarative.yaml", "pkg/api/*.yaml",
		"pkg/api/testdata/*/*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(repoDir, pattern))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	return files
}

// specFamily is the catalogue family of a {type: ...} spec under each key.
var specFamily = map[string]string{
	"iteration": "iterations", "kernel": "kernels", "kernel_a": "kernels", "kernel_b": "kernels",
	"likelihood": "likelihoods", "jump": "jumps", "prior": "priors",
	"termination_condition": "termination_conditions", "timestep_function": "timestep_functions",
	"output_condition": "output_conditions", "condition": "output_conditions",
	"output_function": "output_functions", "execution_strategy": "execution_strategies",
	"environment": "environments", "env": "environments",
}

// usage is one {type: ...} spec found in a config: its family, type and keys,
// and, for an iteration, the params its partition sets and the types of the
// components it takes.
type usage struct {
	at, family, name string
	keys             []string
	params           []string
	components       []string          // family/name
	chosen           map[string]string // field -> the named value it takes
}

// specUsages walks a config's nodes for every {type: ...} spec under a key
// specFamily names, and every macros: entry.
func specUsages(at string, node *yaml3.Node) []usage {
	var found []usage
	var walk func(node, parent *yaml3.Node, key string)
	walk = func(node, parent *yaml3.Node, key string) {
		if node.Kind == yaml3.AliasNode {
			node = node.Alias
		}
		if node.Kind == yaml3.MappingNode {
			if typeNode := mappingValue(node, "type"); typeNode != nil && typeNode.Kind == yaml3.ScalarNode {
				family := specFamily[key]
				if key == "macros" {
					family = "macros"
				}
				if family != "" {
					spec := usage{at: at, family: family, name: typeNode.Value, chosen: map[string]string{}}
					for index := 0; index < len(node.Content); index += 2 {
						if name := node.Content[index].Value; name != "type" {
							spec.keys = append(spec.keys, name)
							if value := node.Content[index+1]; value.Kind == yaml3.ScalarNode {
								spec.chosen[name] = value.Value
							}
						}
					}
					if family == "iterations" && parent != nil {
						for _, wiring := range []string{"params", "params_from_upstream", "params_as_partitions"} {
							if values := mappingValue(parent, wiring); values != nil && values.Kind == yaml3.MappingNode {
								for index := 0; index < len(values.Content); index += 2 {
									spec.params = append(spec.params, values.Content[index].Value)
								}
							}
						}
						spec.components = nestedComponents(node)
					}
					found = append(found, spec)
				}
			}
			for index := 0; index+1 < len(node.Content); index += 2 {
				walk(node.Content[index+1], node, node.Content[index].Value)
			}
			return
		}
		for _, child := range node.Content {
			childKey := key
			if node.Kind == yaml3.DocumentNode {
				childKey = ""
			}
			walk(child, parent, childKey)
		}
	}
	walk(node, nil, "")
	return found
}

// nestedComponents are the family/name of the {type: ...} components an
// iteration's spec takes directly or through another component: its kernel,
// likelihood, jump or nested iteration, which read params of the same
// partition.
func nestedComponents(spec *yaml3.Node) []string {
	var found []string
	for index := 0; index+1 < len(spec.Content); index += 2 {
		family := specFamily[spec.Content[index].Value]
		value := spec.Content[index+1]
		if family == "" || value.Kind != yaml3.MappingNode {
			continue
		}
		if typeNode := mappingValue(value, "type"); typeNode != nil {
			found = append(found, family+"/"+typeNode.Value)
			found = append(found, nestedComponents(value)...)
		}
	}
	return found
}

func TestCatalogueFieldsCoverUsage(t *testing.T) {
	described := catalogueFields()
	var usages []usage
	for _, path := range usageFiles(t) {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var document yaml3.Node
		if err := yaml3.Unmarshal(text, &document); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		usages = append(usages, specUsages(strings.TrimPrefix(path, repoDir+"/"), &document)...)
	}
	// The expansions, which state every macro's partitions in the core
	// language.
	t.Chdir(repoDir)
	for _, path := range oracleConfigs(t) {
		config, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		written, err := ExpandToYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		var document yaml3.Node
		if err := yaml3.Unmarshal(written, &document); err != nil {
			t.Fatal(err)
		}
		usages = append(usages, specUsages("expansion of "+path, &document)...)
	}
	if len(usages) < 300 {
		t.Fatalf("found %d specs in use, want the configs' every one", len(usages))
	}
	listed := func(fields []CatalogueField, name string) bool {
		for _, field := range fields {
			if field.Name == name ||
				(strings.HasSuffix(field.Name, "*") && strings.HasPrefix(name, strings.TrimSuffix(field.Name, "*"))) {
				return true
			}
		}
		return false
	}
	missing := map[string]bool{}
	for _, use := range usages {
		entry, known := described[use.family][use.name]
		if !known {
			continue // registered downstream or behind a build tag: a name only
		}
		for _, key := range use.keys {
			if !listed(entry.Fields, key) {
				missing[fmt.Sprintf("%s %s: field %s (%s)", use.family, use.name, key, use.at)] = true
			}
		}
		incomplete := entry.ParamsIncomplete
		readers := append([]CatalogueField{}, entry.Params...)
		for _, component := range use.components {
			family, name, _ := strings.Cut(component, "/")
			readers = append(readers, described[family][name].Params...)
			incomplete = incomplete || described[family][name].ParamsIncomplete
		}
		for _, field := range entry.Fields {
			for _, choice := range field.Choices {
				if choice.Name == use.chosen[field.Name] {
					readers = append(readers, choice.Params...)
					incomplete = incomplete || choice.ParamsIncomplete
				}
			}
		}
		if incomplete {
			continue
		}
		for _, param := range use.params {
			if strings.Contains(param, "/") {
				continue // routed into a nested run's partition
			}
			if !listed(readers, param) {
				missing[fmt.Sprintf("%s %s: param %s (%s)", use.family, use.name, param, use.at)] = true
			}
		}
	}
	for _, line := range sortedKeys(missing) {
		t.Error(line)
	}
}

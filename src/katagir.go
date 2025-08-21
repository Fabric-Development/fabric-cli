package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

type Repository struct {
	Includes  []Include
	Namespace Namespace
}
type Include struct{ Name, Version string }
type Namespace struct {
	Name           string
	Classes        []Class
	Enums          []Enumeration
	Functions      []Function
	Constants      []Constant
	Imports        []string
	ForeignImports []string
}
type Constant struct {
	Name, Value, Doc string
	Type             Type
}
type Class struct {
	Name           string
	Parent         string
	Implements     []Implement
	Doc            string
	Methods        []Function
	VirtualMethods []Function
	Functions      []Function
	Constructor    Function
	Properties     []Property
}
type Implement struct{ Name string }
type Property struct {
	Name, Doc string
	Type      Type
}
type Enumeration struct {
	Name, Doc string
	Members   []Member
}
type Member struct{ Name, Value, Doc string }
type Function struct {
	Name, Doc   string
	ReturnValue ReturnValue
	Parameters  Parameters
}
type ReturnValue struct {
	Doc  string
	Type Type
}
type Parameters struct{ InstanceParameters, Parameters []Parameter }
type Parameter struct {
	Name, Doc, Nullable string
	Type                Type
}
type Type struct{ Name string }

var pythonKeywords = map[string]struct{}{"and": {}, "as": {}, "assert": {}, "async": {}, "await": {}, "break": {}, "class": {}, "continue": {}, "def": {}, "del": {}, "elif": {}, "else": {}, "except": {}, "False": {}, "finally": {}, "for": {}, "from": {}, "global": {}, "if": {}, "import": {}, "in": {}, "is": {}, "lambda": {}, "None": {}, "nonlocal": {}, "not": {}, "or": {}, "pass": {}, "raise": {}, "return": {}, "True": {}, "try": {}, "while": {}, "with": {}, "yield": {}}
var knownForeignModules = map[string]struct{}{"cairo": {}, "xlib": {}}

func pytype(t string) string {
	switch t {
	case "gboolean":
		return "bool"
	case "gint", "guint", "gint8", "guint8", "gint16", "guint16", "gint32", "guint32", "gint64", "guint64", "gsize", "gshort", "gushort":
		return "int"
	case "gfloat", "gdouble":
		return "float"
	case "gchar", "guchar", "utf8", "GString", "filename":
		return "str"
	case "gpointer":
		return "typing.Any"
	case "none", "":
		return "None"
	}
	if strings.Contains(t, ".") {
		parts := strings.Split(t, ".")
		return fmt.Sprintf("%s.%s", parts[0], parts[1])
	}
	return fmt.Sprintf("\"%s\"", t)
}

func pyvalue(val string, typeName string) string {
	if val == "(null)" {
		return "None"
	}
	if pytype(typeName) == "str" && !strings.HasPrefix(val, "\"") {
		return fmt.Sprintf("\"%s\"", val)
	}
	return val
}

func pydoc(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	if strings.Contains(s, "\n") {
		return `"""` + "\n" + s + "\n" + `"""`
	}
	return `"""` + s + `"""`
}

func snakecase(s string) string { return strings.ReplaceAll(s, "-", "_") }

func safeName(s string) string {
	name := snakecase(s)
	if _, isKeyword := pythonKeywords[name]; isKeyword {
		return name + "_"
	}
	return name
}

func pyparams(p Parameters) string {
	var params []string
	firstOptionalIdx := -1
	for i, param := range p.Parameters {
		if param.Nullable == "1" {
			firstOptionalIdx = i
			break
		}
	}
	for i, param := range p.Parameters {
		if param.Name == "..." {
			params = append(params, "*args: typing.Any")
			continue
		}
		pname := safeName(param.Name)
		ptype := pytype(param.Type.Name)
		if param.Nullable == "1" {
			ptype = fmt.Sprintf("typing.Optional[%s]", ptype)
		}
		if firstOptionalIdx != -1 && i >= firstOptionalIdx {
			params = append(params, fmt.Sprintf("%s: %s = None", pname, ptype))
		} else {
			params = append(params, fmt.Sprintf("%s: %s", pname, ptype))
		}
	}
	return strings.Join(params, ", ")
}

func pybase(b string) string {
	if b == "" {
		return "GObject.Object"
	}
	if strings.Contains(b, ".") {
		parts := strings.Split(b, ".")
		return fmt.Sprintf("%s.%s", parts[0], parts[1])
	}
	return b
}

var templateFuncs = template.FuncMap{"pytype": pytype, "pyvalue": pyvalue, "pydoc": pydoc, "upper": strings.ToUpper, "safeName": safeName, "pyparams": pyparams, "pybase": pybase, "join": func(sep string, s []string) string { return strings.Join(s, sep) }}

const pyiTemplate = `import typing
import enum
{{if .ForeignImports}}
{{range .ForeignImports}}
import {{.}}
{{- end}}

{{end}}from gi.repository import {{.Imports | join ", "}}
{{range .Constants}}
{{.Name | safeName}}: typing.Final[{{.Type.Name | pytype}}] = {{pyvalue .Value .Type.Name}}
{{end}}{{range .Enums}}
class {{.Name}}(enum.Enum):
    {{.Doc | pydoc}}
    {{range .Members}}
    {{.Name | upper}} = {{.Value}}
    {{end}}
{{end}}{{range .Functions}}
def {{.Name | safeName}}({{.Parameters | pyparams}}) -> {{.ReturnValue.Type.Name | pytype}}:
    {{.Doc | pydoc}}
    ...
{{end}}{{range .Classes}}
class {{.Name}}({{.Parent | pybase}}):
    {{.Doc | pydoc}}
    {{if .Properties}}
    class props{{if .Parent}}({{.Parent | pybase}}.props){{end}}:
        {{range .Properties}}
        {{.Name | safeName}}: {{.Type.Name | pytype}}
        {{end}}
    {{end}}
    def __init__(self, *args, **kwargs) -> None:
        ...
    {{range .Methods}}
    def {{.Name | safeName}}(self{{if .Parameters.Parameters}}, {{end}}{{.Parameters | pyparams}}) -> {{.ReturnValue.Type.Name | pytype}}:
        {{.Doc | pydoc}}
        ...
    {{end}}
    {{range .Functions}}
    @staticmethod
    def {{.Name | safeName}}({{.Parameters | pyparams}}) -> {{.ReturnValue.Type.Name | pytype}}:
        {{.Doc | pydoc}}
        ...
    {{end}}
{{end}}
`

type Generator struct {
	outDir           string
	noDeps           bool
	processedModules map[string]bool
	tmpl             *template.Template
}

func (g *Generator) doGenerate(module string) error {
	parts := strings.Split(module, "-")
	if len(parts) != 2 {
		return fmt.Errorf("invalid module format: %s (expected <NAME>-<VERSION>)", module)
	}
	name, version := parts[0], parts[1]

	moduleKey := fmt.Sprintf("%s-%s", name, version)
	if g.processedModules[moduleKey] {
		return nil
	}
	g.processedModules[moduleKey] = true

	fmt.Printf("processing %s-%s\n", name, version)

	repo, err := parseGirFile(name, version)
	if err != nil {
		return err
	}

	if !g.noDeps {
		for _, include := range repo.Includes {
			if err := g.doGenerate(fmt.Sprintf("%s-%s", include.Name, include.Version)); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not process dependency %s: %v\n", include.Name, err)
			}
		}
	}

	imports := map[string]struct{}{"GLib": {}, "GObject": {}, "Gio": {}}
	foreignImports := make(map[string]struct{})
	collectImports(repo, imports, foreignImports)
	delete(imports, name) // do not import self (happens sometimes)

	ns := &repo.Namespace
	ns.Imports = setToSlice(imports)
	ns.ForeignImports = setToSlice(foreignImports)

	transformNamespace(ns)

	return g.writeStubFile(name, ns)
}

func collectImports(repo *Repository, imports, foreignImports map[string]struct{}) {
	// explicit dependencies from <include/> tags
	for _, include := range repo.Includes {
		if _, isForeign := knownForeignModules[include.Name]; isForeign {
			foreignImports[include.Name] = struct{}{}
		} else {
			imports[include.Name] = struct{}{}
		}
	}

	// dependencies found by scanning types
	addImplicit := func(typeName string) {
		if !strings.Contains(typeName, ".") {
			return
		}
		namespace := strings.Split(typeName, ".")[0]
		if _, isForeign := knownForeignModules[namespace]; isForeign {
			foreignImports[namespace] = struct{}{}
		} else {
			imports[namespace] = struct{}{}
		}
	}

	for _, c := range repo.Namespace.Classes {
		addImplicit(c.Parent)
		for _, impl := range c.Implements {
			addImplicit(impl.Name)
		}
		for _, prop := range c.Properties {
			addImplicit(prop.Type.Name)
		}
		allFuncs := append(c.Methods, c.Functions...)
		allFuncs = append(allFuncs, c.VirtualMethods...)
		for _, f := range allFuncs {
			addImplicit(f.ReturnValue.Type.Name)
			for _, p := range f.Parameters.Parameters {
				addImplicit(p.Type.Name)
			}
		}
	}
	for _, f := range repo.Namespace.Functions {
		addImplicit(f.ReturnValue.Type.Name)
		for _, p := range f.Parameters.Parameters {
			addImplicit(p.Type.Name)
		}
	}
}

func parseGirFile(name, version string) (*Repository, error) {
	path := filepath.Join(GetGirLookupPath(), fmt.Sprintf("%s-%s.gir", name, version))
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open GIR file %s: %w", path, err)
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	decoder.Entity = xml.HTMLEntity

	var repo Repository
	var currentClass *Class
	var currentFunc *Function
	var currentEnum *Enumeration
	var currentMember *Member
	var currentProperty *Property
	var currentParam *Parameter
	var currentReturn *ReturnValue
	var parentOfDoc string

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			elementName := t.Name.Local
			switch elementName {
			case "repository", "namespace": // toplevels
			case "include":
				if t.Name.Space != "http://www.gtk.org/introspection/c/1.0" {
					repo.Includes = append(repo.Includes, Include{Name: getAttr(t, "name"), Version: getAttr(t, "version")})
				}
			case "class", "interface", "record":
				class := Class{Name: getAttr(t, "name"), Parent: getAttr(t, "parent")}
				repo.Namespace.Classes = append(repo.Namespace.Classes, class)
				currentClass = &repo.Namespace.Classes[len(repo.Namespace.Classes)-1]
				parentOfDoc = elementName
			case "implements":
				if currentClass != nil {
					currentClass.Implements = append(currentClass.Implements, Implement{Name: getAttr(t, "name")})
				}
			case "enumeration", "bitfield":
				enum := Enumeration{Name: getAttr(t, "name")}
				repo.Namespace.Enums = append(repo.Namespace.Enums, enum)
				currentEnum = &repo.Namespace.Enums[len(repo.Namespace.Enums)-1]
				parentOfDoc = elementName
			case "member":
				if currentEnum != nil {
					member := Member{Name: getAttr(t, "name"), Value: getAttr(t, "value")}
					currentEnum.Members = append(currentEnum.Members, member)
					currentMember = &currentEnum.Members[len(currentEnum.Members)-1]
					parentOfDoc = elementName
				}
			case "method", "virtual-method", "function", "constructor":
				fn := Function{Name: getAttr(t, "name")}
				if currentClass != nil {
					switch elementName {
					case "method":
						currentClass.Methods = append(currentClass.Methods, fn)
						currentFunc = &currentClass.Methods[len(currentClass.Methods)-1]
					case "virtual-method":
						currentClass.VirtualMethods = append(currentClass.VirtualMethods, fn)
						currentFunc = &currentClass.VirtualMethods[len(currentClass.VirtualMethods)-1]
					case "function":
						currentClass.Functions = append(currentClass.Functions, fn)
						currentFunc = &currentClass.Functions[len(currentClass.Functions)-1]
					case "constructor":
						currentClass.Constructor = fn
						currentFunc = &currentClass.Constructor
					}
				} else {
					repo.Namespace.Functions = append(repo.Namespace.Functions, fn)
					currentFunc = &repo.Namespace.Functions[len(repo.Namespace.Functions)-1]
				}
				parentOfDoc = elementName
			case "return-value":
				if currentFunc != nil {
					currentReturn = &currentFunc.ReturnValue
					parentOfDoc = elementName
				}
			case "property":
				if currentClass != nil {
					prop := Property{Name: getAttr(t, "name")}
					currentClass.Properties = append(currentClass.Properties, prop)
					currentProperty = &currentClass.Properties[len(currentClass.Properties)-1]
					parentOfDoc = elementName
				}
			case "parameter":
				if currentFunc != nil {
					param := Parameter{Name: getAttr(t, "name"), Nullable: getAttr(t, "nullable")}
					currentFunc.Parameters.Parameters = append(currentFunc.Parameters.Parameters, param)
					currentParam = &currentFunc.Parameters.Parameters[len(currentFunc.Parameters.Parameters)-1]
					parentOfDoc = elementName
				}
			case "type":
				typeName := getAttr(t, "name")
				switch {
				case currentParam != nil:
					currentParam.Type.Name = typeName
				case currentReturn != nil:
					currentReturn.Type.Name = typeName
				case currentProperty != nil:
					currentProperty.Type.Name = typeName
				}
			}
		case xml.CharData:
			content := strings.TrimSpace(string(t))
			if content == "" {
				continue
			}
			switch parentOfDoc {
			case "class", "interface", "record":
				if currentClass != nil {
					currentClass.Doc = content
				}
			case "method", "virtual-method", "function", "constructor":
				if currentFunc != nil {
					currentFunc.Doc = content
				}
			case "enumeration", "bitfield":
				if currentEnum != nil {
					currentEnum.Doc = content
				}
			case "member":
				if currentMember != nil {
					currentMember.Doc = content
				}
			case "property":
				if currentProperty != nil {
					currentProperty.Doc = content
				}
			case "parameter":
				if currentParam != nil {
					currentParam.Doc = content
				}
			case "return-value":
				if currentReturn != nil {
					currentReturn.Doc = content
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "class", "interface", "record":
				currentClass = nil
			case "method", "virtual-method", "function", "constructor":
				currentFunc = nil
			case "enumeration", "bitfield":
				currentEnum = nil
			case "member":
				currentMember = nil
			case "property":
				currentProperty = nil
			case "parameter":
				currentParam = nil
			case "return-value":
				currentReturn = nil
			}
		}
	}
	return &repo, nil
}

// transform symbols from the GIR to meet python's conventions
func transformNamespace(ns *Namespace) {
	for i := range ns.Classes {
		class := &ns.Classes[i]
		if len(class.VirtualMethods) > 0 {
			for _, vmethod := range class.VirtualMethods {
				vmethod.Name = "do_" + vmethod.Name
				class.Methods = append(class.Methods, vmethod)
			}
			class.VirtualMethods = nil
		}
	}
	ns.Classes = topoSortClasses(ns.Classes)
	for i := range ns.Enums {
		enum := &ns.Enums[i]
		if len(enum.Members) < 2 {
			continue
		}
		memberNames := make([]string, len(enum.Members))
		for j, member := range enum.Members {
			memberNames[j] = member.Name
		}
		prefix := longestCommonPrefix(memberNames)
		if lastUnderscore := strings.LastIndex(prefix, "_"); lastUnderscore != -1 {
			prefix = prefix[:lastUnderscore+1]
			for j := range enum.Members {
				enum.Members[j].Name = strings.TrimPrefix(enum.Members[j].Name, prefix)
			}
		}
	}
}

func (g *Generator) writeStubFile(name string, ns *Namespace) error {
	path := filepath.Join(g.outDir, "repository", name+".pyi")
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create output file %s: %w", path, err)
	}
	defer file.Close()
	if err := g.tmpl.Execute(file, ns); err != nil {
		return fmt.Errorf("failed to execute template for %s: %w", name, err)
	}
	fmt.Printf("successfully generated stub for %s: %s\n", name, path)
	return nil
}

func createStubTree(dir string) error {
	repo := filepath.Join(dir, "repository")
	if err := os.MkdirAll(repo, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "py.typed"), []byte("partial\n"), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "__init__.pyi"), []byte(""), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repo, "__init__.pyi"), []byte(""), 0644)
}

func topoSortClasses(classes []Class) []Class {
	classNameToClass := make(map[string]Class)
	graph := make(map[string][]string)
	inDegree := make(map[string]int)
	for _, c := range classes {
		classNameToClass[c.Name] = c
		inDegree[c.Name] = 0
	}
	for _, c := range classes {
		dependencies := []string{c.Parent}
		for _, impl := range c.Implements {
			dependencies = append(dependencies, impl.Name)
		}
		for _, depName := range dependencies {
			if _, exists := classNameToClass[depName]; exists {
				graph[depName] = append(graph[depName], c.Name)
				inDegree[c.Name]++
			}
		}
	}

	queue := make([]string, 0)
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	sortedClasses := make([]Class, 0, len(classes))
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		sortedClasses = append(sortedClasses, classNameToClass[name])
		for _, neighbor := range graph[name] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}
	if len(sortedClasses) != len(classes) && len(classes) > 0 {
		fmt.Fprintf(os.Stderr, "warning: cycle detected or missing base class in hierarchy\n")
	}
	return sortedClasses
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for i := 1; i < len(strs); i++ {
		for !strings.HasPrefix(strs[i], prefix) {
			if len(prefix) == 0 {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func getAttr(t xml.StartElement, name string) string {
	for _, attr := range t.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func setToSlice(set map[string]struct{}) []string {
	slice := make([]string, 0, len(set))
	for item := range set {
		slice = append(slice, item)
	}
	sort.Strings(slice)
	return slice
}

func GetGirLookupPath() string {
	const defaultGirLookupPath = "/usr/share/gir-1.0/"
	if value, ok := os.LookupEnv("GIR_LOOKUP_PATH"); ok {
		return value
	}
	return defaultGirLookupPath
}

func GenerateStubs(modules []string, outDir string, noDeps bool) {
	if len(modules) == 0 {
		fmt.Println("provide at least one module (e.g., Gtk-3.0)")
		os.Exit(1)
	}

	if err := createStubTree(outDir); err != nil {
		fmt.Printf("error creating package stub tree: %v\n", err)
		os.Exit(1)
	}

	tmpl, err := template.New("pyi").Funcs(templateFuncs).Parse(pyiTemplate)
	if err != nil {
		fmt.Printf("error initializing template: %v\n", err)
		os.Exit(1)
	}

	gen := &Generator{
		outDir:           outDir,
		noDeps:           noDeps,
		processedModules: make(map[string]bool),
		tmpl:             tmpl,
	}

	for _, module := range modules {
		if err := gen.doGenerate(module); err != nil {
			fmt.Printf("error processing %s: %v\n", module, err)
		}
	}
}

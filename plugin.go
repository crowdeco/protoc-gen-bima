package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"regexp"
	"strings"

	version "github.com/crowdeco/protoc-gen-bima/internal"
	gorm "github.com/crowdeco/protoc-gen-bima/options"
	"github.com/iancoleman/strcase"
	"golang.org/x/mod/modfile"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	// "google.golang.org/protobuf/types/descriptorpb"
)

var GenerateVersionMarkers = true
var ptypesImport = "github.com/golang/protobuf/ptypes"

type structFields = map[string]string

var basicTypes = map[string]struct{}{
	"bool": {},
	"int":  {},
	"int8": {}, "int16": {},
	"int32": {}, "int64": {},
	"uint":  {},
	"uint8": {}, "uint16": {},
	"uint32": {}, "uint64": {},
	"uintptr": {},
	"float32": {}, "float64": {},
	"string": {},
	"[]byte": {},
}

var wellKnownTypes = map[string]string{
	"DoubleValue": "float64",
	"FloatValue":  "float32",
	"Int64Value":  "int64",
	"UInt64Value": "uint64",
	"Int32Value":  "int32",
	"UInt32Value": "uint32",
	"BoolValue":   "bool",
	"StringValue": "string",
	"BytesValue":  "[]byte", // * this was commented in origin
}

var sqlTypes = map[string]string{
	"sql.NullString":  "String",
	"sql.NullInt64":   "Int64",
	"sql.NullInt32":   "Int32",
	"sql.NullFloat64": "Float64",
	"sql.NullBool":    "Bool",
	"sql.NullTime":    "Time",
}

var status200 = []string{"StatusOK", "StatusCreated", "StatusNoContent"}
var status400 = []string{"StatusBadRequest", "StatusNotFound"}

type BimaPlugin struct {
	*protogen.Plugin
	files             map[string]*fileInfo
	modelExports      map[string]bool
	modelTypes        map[string]structFields
	packageName       string
	loggerHasDeclared bool
}

func (p BimaPlugin) Generate(plugin *protogen.Plugin) {
	p.init(plugin)
	p.findMarkedFiles()
	for _, f := range p.files {
		p.generateFile(f)
	}
}

func (p *BimaPlugin) init(plugin *protogen.Plugin) {
	p.Plugin = plugin
	if p.files == nil {
		p.files = make(map[string]*fileInfo)
	}
	if p.modelExports == nil {
		p.modelExports = make(map[string]bool)
	}
	if p.modelTypes == nil {
		p.modelTypes = make(map[string]structFields)
	}
	p.packageName = getPackageName()
	if p.packageName == "" {
		println("Warning: go.mod not found")
	}
}

func (p *BimaPlugin) findMarkedFiles() {
	for _, f := range p.Files {
		if !f.Generate {
			continue
		}
		for _, m := range f.Messages {
			p.inspect(f, m.Desc)
		}
	}
}

func (p *BimaPlugin) inspect(f *protogen.File, m protoreflect.MessageDescriptor) {
	if opts := getMessageOptions(m); opts != nil {
		if _, exists := p.files[*f.Proto.Name]; !exists {
			hasTimestamp := false
			for _, dep := range f.Proto.Dependency {
				if dep == "google/protobuf/timestamp.proto" {
					hasTimestamp = true
				}
			}
			p.files[*f.Proto.Name] = newFileInfo(f, hasTimestamp)
		}
	}
}

func (p *BimaPlugin) generateFile(f *fileInfo) {
	file := f.File
	filename := file.GeneratedFilenamePrefix + ".pb.bima.go"
	g := p.NewGeneratedFile(filename, file.GoImportPath)

	p.genGeneratedHeader(g, file)

	g.P("package ", file.GoPackageName)
	g.P()

	for i, imps := 0, file.Desc.Imports(); i < imps.Len(); i++ {
		p.genImport(g, file, imps.Get(i))
	}

	// got import cycle
	// if !p.loggerHasDeclared {
	// 	g.QualifiedGoIdent(protogen.GoIdent{
	// 		GoImportPath: protogen.GoImportPath(p.packageName + "/generated/dic"),
	// 	})
	// 	g.P("var container, _ = dic.NewContainer()")
	// 	g.P("var logger = container.GetBimaHandlerLogger()")
	// 	g.P()
	// 	p.loggerHasDeclared = true
	// }

	reResponse := regexp.MustCompile(`Response$`)
	for _, m := range file.Messages {
		if mi, ok := getModelIdent(m.Desc); ok {
			p.genWeakTimestamp(g, f)
			p.genModelExport(g, mi)
			p.genBindFunc(g, m, mi)
			p.genBundleFunc(g, m, mi)
		}
		p.genResponseStatusMethod(g, m, file.Messages)
		if reResponse.MatchString(m.GoIdent.GoName) {
			p.genResponseStatusFunc(g, m)
		}
	}
}

func (p *BimaPlugin) genGeneratedHeader(g *protogen.GeneratedFile, f *protogen.File) {
	g.P("// Code generated by protoc-gen-bima. DO NOT EDIT.")

	if GenerateVersionMarkers {
		g.P("// versions:")
		protocGenBimaVersion := version.String()
		protocVersion := "(unknown)"
		if v := p.Request.GetCompilerVersion(); v != nil {
			protocVersion = fmt.Sprintf("v%v.%v.%v", v.GetMajor(), v.GetMinor(), v.GetPatch())
		}
		g.P("// \tprotoc-gen-bima ", protocGenBimaVersion)
		g.P("// \tprotoc          ", protocVersion)
		g.P("// source: ", f.Desc.Path())
	}

	g.P()
}

func (p *BimaPlugin) genImport(g *protogen.GeneratedFile, file *protogen.File, imp protoreflect.FileImport) {
	impFile, ok := p.FilesByPath[imp.Path()]
	if !ok {
		return
	}
	if impFile.GoImportPath == file.GoImportPath {
		return
	}
	if !imp.IsWeak {
		g.Import(impFile.GoImportPath)
	}
	if !imp.IsPublic {
		return
	}
	g.P()
}

func (p *BimaPlugin) genWeakTimestamp(g *protogen.GeneratedFile, f *fileInfo) {
	if f.hasTimestamp {
		g.P("type _ timestamp.Timestamp")
		g.P()
	}
}

func (p *BimaPlugin) genModelExport(g *protogen.GeneratedFile, model protogen.GoIdent) {
	if !p.modelExports[model.GoName] {
		g.P("type ", model.GoName, "Model = ", model) // * e.g TodoModel
		g.P()
		p.modelExports[model.GoName] = true
	}
}

func (p *BimaPlugin) genBindFunc(g *protogen.GeneratedFile, m *protogen.Message, model protogen.GoIdent) {
	if p.walkModelFields(model) {
		g.P("func (x *", m.GoIdent, ") Bind(v *", model, ") {")
		g.P("to, from := v, x")
		for _, f := range m.Fields {
			p.genFieldConversion(g, m, f, model, false)
		}
		g.P("}")
		g.P()
		p.genToModelFunc(g, m, model)
	}
}

func (p *BimaPlugin) genToModelFunc(g *protogen.GeneratedFile, m *protogen.Message, model protogen.GoIdent) {
	g.P("func (x *", m.GoIdent, ") ToModel() ", model, " {")
	g.P("v := ", model, "{}")
	g.P("x.Bind(&v)")
	g.P("return v")
	g.P("}")
	g.P()
}

func (p *BimaPlugin) genBundleFunc(g *protogen.GeneratedFile, m *protogen.Message, model protogen.GoIdent) {
	if p.walkModelFields(model) {
		g.P("func (x *", m.GoIdent, ") Bundle(v *", model, ") {")
		g.P("to, from := x, v")
		for _, f := range m.Fields {
			p.genFieldConversion(g, m, f, model, true)
		}
		g.P("}")
		g.P()
	}
}

func (p *BimaPlugin) genResponseStatusMethod(g *protogen.GeneratedFile, m *protogen.Message, ms []*protogen.Message) {
	reResponse := regexp.MustCompile(`Response$`)
	for _, msg := range ms {
		if reResponse.MatchString(msg.GoIdent.GoName) {
			for _, field := range msg.Fields {
				if field.Desc.Name() == "data" && field.Message == m && !field.Desc.IsList() {
					g.QualifiedGoIdent(protogen.GoIdent{
						GoImportPath: "net/http",
					})
					for _, status := range status200 {
						g.P("func (x *", m.GoIdent, ") ", msg.GoIdent, status, "() (*", msg.GoIdent, ", error) {")
						g.P("return &", msg.GoIdent, "{")
						g.P("Code: http.", status, ",")
						if status != "StatusNoContent" {
							g.P("Data: x,")
						}
						g.P("}, nil")
						g.P("}")
						g.P()
					}
					for _, status := range status400 {
						g.P("func (x *", m.GoIdent, ") ", msg.GoIdent, status, "(err error) (*", msg.GoIdent, ", error) {")
						g.P("return &", msg.GoIdent, "{")
						g.P("Code: http.", status, ",")
						g.P("Data: x,")
						g.P("Message: err.Error(),")
						g.P("}, nil")
						g.P("}")
						g.P()
					}
				}
			}
		}
	}
}

func (p *BimaPlugin) genResponseStatusFunc(g *protogen.GeneratedFile, m *protogen.Message) {
	rePaginatedResponse := regexp.MustCompile(`PaginatedResponse$`)
	for _, field := range m.Fields {
		// TODO: support paginate too
		if field.Desc.Name() == "data" {
			if field.Message != nil {
				p := "*"
				if field.Desc.IsList() {
					p = "[]*"
				}
				g.QualifiedGoIdent(protogen.GoIdent{
					GoImportPath: "net/http",
				})
				for _, status := range status200 {
					g.P("func ", m.GoIdent, status, "(d ", p, field.Message.GoIdent, ") (*", m.GoIdent, ", error) {")
					g.P("return &", m.GoIdent, "{")
					g.P("Code: http.", status, ",")
					if status != "StatusNoContent" {
						g.P("Data: d,")
					}
					g.P("}, nil")
					g.P("}")
					g.P()
				}
				for _, status := range status400 {
					g.P("func ", m.GoIdent, status, "(d ", p, field.Message.GoIdent, ", err error) (*", m.GoIdent, ", error) {")
					g.P("return &", m.GoIdent, "{")
					g.P("Code: http.", status, ",")
					g.P("Data: d,")
					if !rePaginatedResponse.MatchString(m.GoIdent.GoName) {
						g.P("Message: err.Error(),")
					}
					g.P("}, nil")
					g.P("}")
					g.P()
				}
			}
		}
	}
}

func (p *BimaPlugin) walkModelFields(model protogen.GoIdent) bool {
	// * assume filename is snake case
	filename := string(model.GoImportPath) + "/" + strcase.ToSnake(model.GoName) + ".go"
	origin := filename
	if _, ok := p.modelTypes[model.GoName]; ok {
		return true
	}

	fset := token.NewFileSet()

	var astFile *ast.File
	var err error

	for astFile == nil && len(strings.Split(filename, "/")) > 1 {
		if astFile, err = parser.ParseFile(fset, filename, nil, parser.ParseComments); astFile != nil {
			break
		}
		if i := strings.Index(filename, "/"); i >= 0 {
			filename = filename[i+1:]
		} else {
			break
		}
	}

	if err != nil {
		p.Error(err)
		return false
	}

	for _, decl := range astFile.Decls {
		switch decl := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					switch st := spec.Type.(type) {
					case *ast.StructType:
						if spec.Name.Name == model.GoName {
							sf := structFields{}
							// * exclude embedded struct e.g bima.Model

							for _, field := range st.Fields.List {
								var fieldName string
								// * a,b type ; a type ; not configs.Base
								for _, name := range field.Names {
									fieldName = name.Name
									switch ft := field.Type.(type) {
									case *ast.Ident:
										sf[fieldName] = ft.Name
									case *ast.StarExpr:
										switch sft := ft.X.(type) {
										case *ast.Ident:
											sf[fieldName] = "*" + sft.Name
										case *ast.SelectorExpr:
											sf[fieldName] = "*" + sft.X.(*ast.Ident).Name + "." + sft.Sel.Name
										}
									case *ast.SelectorExpr:
										sf[fieldName] = ft.X.(*ast.Ident).Name + "." + ft.Sel.Name
										// case *ast.ArrayType:
										// 	switch fte := ft.Elt.(type) {
										// 	case *ast.Ident:
										// 		sf[fieldName] = "[]" + fte.Name
										// 	case *ast.StarExpr:
										// 		sf[fieldName] = "[]*" + fte.X.(*ast.Ident).Name
										// 	}
									}
								}
							}
							p.modelTypes[model.GoName] = sf
							return true
						}
					}
				}
			}
		}
	}

	println(fmt.Sprintf("couldn't find type %s in %s", model.GoName, origin))
	return false
}

func (p *BimaPlugin) genFieldConversion(g *protogen.GeneratedFile, m *protogen.Message, field *protogen.Field, model protogen.GoIdent, toX bool) {
	fieldType, pointer := fieldGoType(g, field)
	if pointer {
		fieldType = "*" + fieldType
	}

	fieldName := field.GoName

	structFields, ok := p.modelTypes[model.GoName]
	if !ok {
		p.Error(errors.New("Something went wrong, please ask the author about this error"))
		return
	}

	if field.Desc.IsList() {
		// TODO
	} else if field.Desc.Message() != nil {
		parts := strings.Split(fieldType, ".")
		pbType := parts[len(parts)-1]
		typeStr, exists := structFields[fieldName]

		if exists {
			coreType, pointer := parseType(typeStr)
			if pbFieldType, ok := wellKnownTypes[pbType]; ok {
				if toX {
					if v, ok := sqlTypes[coreType]; ok {
						if pointer {
							p.Error(errors.New(fmt.Sprintf("type %s is not supported", typeStr)))
							return
						}
						g.P("if from.", fieldName, ".Valid {")
						g.P("to.", fieldName, " = &", fieldType[1:], "{Value: from.", fieldName, ".", v, "}")
						g.P("}")
					} else {
						if pointer {
							g.P("if from.", fieldName, " != nil {")
							g.P("to.", fieldName, " = &", fieldType[1:], "{Value: *from.", fieldName, "}")
							g.P("}")
						} else {
							println(fmt.Sprintf("Warning: Please change protobuf type message's field regarding type of %s on model %s", fieldName, model.GoName))
							// TODO: should we generate ?
						}
					}
				} else {
					if v, ok := sqlTypes[coreType]; ok {
						if pointer {
							p.Error(errors.New(fmt.Sprintf("type %s is not supported", typeStr)))
							return
						}
						g.P("if from.", fieldName, " != nil {")
						r := "from." + fieldName + ".Value"
						g.QualifiedGoIdent(protogen.GoIdent{
							GoImportPath: "database/sql",
						})
						r = coreType + "{" + v + ": " + r + ", Valid: true}"
						g.P("to.", fieldName, " = ", r)
						g.P("}")
					} else {
						if pointer {
							g.P("if from.", fieldName, " != nil {")
							r := "from." + fieldName + ".Value"
							switch coreType {
							case "string", "bool":
								r = "&" + r
							default:
								if pbFieldType == coreType {
									r = "&" + r
								} else {
									g.P(toLowerFirst(fieldName), " := ", coreType, "(", r, ")")
									r = "&" + toLowerFirst(fieldName)
								}
							}
							g.P("to.", fieldName, " = ", r)
							g.P("}")
						} else {
							println(fmt.Sprintf("Warning: Please change protobuf type message's field regarding type of %s on model %s", fieldName, model.GoName))
							// TODO: should we generate ?
						}
					}

				}
			} else if pbType == "Timestamp" {
				if toX {
					g.QualifiedGoIdent(protogen.GoIdent{
						GoImportPath: protogen.GoImportPath(ptypesImport),
					})
					if _, ok := sqlTypes[coreType]; ok {
						if coreType != "sql.NullTime" {
							p.Error(errors.New(fmt.Sprintf("type %s is not to be used for Timestamp", typeStr)))
							return
						}
						if pointer {
							p.Error(errors.New(fmt.Sprintf("type %s is not supported", typeStr)))
							return
						}
						g.P("if from.", fieldName, ".Valid {")
						g.P("to.", fieldName, ", _ = ", "ptypes.TimestampProto(from.", fieldName, ".Time)")
						g.P("}")
					} else {
						if coreType != "time.Time" {
							p.Error(errors.New(fmt.Sprintf("type %s is not to be used for Timestamp", typeStr)))
							return
						}
						if pointer {
							g.P("if from.", fieldName, " != nil {")
							g.P("to.", fieldName, ", _ = ", "ptypes.TimestampProto(*from.", fieldName, ")")
							g.P("}")
						} else {
							g.P("to.", fieldName, ", _ = ", "ptypes.TimestampProto(from.", fieldName, ")")
						}
					}
				} else {
					g.P("if from.", fieldName, " != nil {")
					g.P("if from.", fieldName, ".IsValid() {")
					if _, ok := sqlTypes[coreType]; ok {
						if coreType != "sql.NullTime" {
							p.Error(errors.New(fmt.Sprintf("type %s is not to be used for Timestamp", typeStr)))
							return
						}
						if pointer {
							p.Error(errors.New(fmt.Sprintf("type %s is not supported", typeStr)))
							return
						}
						g.QualifiedGoIdent(protogen.GoIdent{
							GoImportPath: "database/sql",
						})
						g.P("to.", fieldName, " = ", coreType, "{Time: from.", fieldName, ".AsTime(), Valid: true}")
					} else {
						if coreType != "time.Time" {
							p.Error(errors.New(fmt.Sprintf("type %s is not to be used for Timestamp", typeStr)))
							return
						}
						if pointer {
							g.P("t := from.", fieldName, ".AsTime()")
							g.P("to.", fieldName, " = &t")
						} else {
							g.P("to.", fieldName, " = from.", fieldName, ".AsTime()")
						}
					}
					g.P("}")
					g.P("}")
				}
			} else {
				// TODO
			}
		}
	} else {
		typeStr, exists := structFields[fieldName]
		if exists {
			coreType, pointer := parseType(typeStr)
			switch coreType {
			case "string", "bool":
				if pointer {
					if toX {
						g.P("to.", fieldName, " = *from.", fieldName)
					} else {
						g.P("to.", fieldName, " = &from.", fieldName)
					}
				} else {
					g.P("to.", fieldName, " = from.", fieldName)
				}
			default:
				if fieldType == coreType {
					g.P("to.", fieldName, " = from.", fieldName)
				} else {
					if v, ok := sqlTypes[coreType]; ok {
						if v == "Time" {
							v = "time.Time"
						}
						println(fmt.Sprintf("Warning: Please change field %s on model %s to %s type, it wouldn't benefit anything", fieldName, model.GoName, toLowerFirst(v)))
						// TODO: should we generate ?
					} else {
						if toX {
							if pointer {
								g.P("to.", fieldName, " = ", fieldType, "(*from.", fieldName, ")")
							} else {
								g.P("to.", fieldName, " = ", fieldType, "(from.", fieldName, ")")
							}
						} else {
							fieldType = coreType
							if pointer {
								g.P(toLowerFirst(fieldName), " := ", fieldType, "(from.", fieldName, ")")
								g.P("to.", fieldName, " = &", toLowerFirst(fieldName))
							} else {
								g.P("to.", fieldName, " = ", fieldType, "(from.", fieldName, ")")
							}
						}
					}
				}
			}
		} else if fieldName == "Id" {
			// * always string
			g.P("to.Id = from.Id")
		}
	}
}

func getMessageOptions(m protoreflect.MessageDescriptor) *gorm.GormMessageOptions {
	if m.Options() == nil {
		return nil
	}
	if !proto.HasExtension(m.Options(), gorm.E_Opts) {
		return nil
	}
	ext := proto.GetExtension(m.Options(), gorm.E_Opts)
	opts, ok := ext.(*gorm.GormMessageOptions)
	if !ok {
		println(fmt.Sprintf("extension is %T; want an GormMessageOptions", ext))
		return nil
	}
	return opts
}

func getModelIdent(md protoreflect.MessageDescriptor) (protogen.GoIdent, bool) {
	if opt := getMessageOptions(md).GetModel(); opt != "" {
		if i := strings.Index(opt, ";"); i >= 0 {
			return protogen.GoIdent{
				GoName:       opt[i+1:],
				GoImportPath: protogen.GoImportPath(opt[:i]),
			}, true
		}
	}
	return protogen.GoIdent{}, false
}

func fieldGoType(g *protogen.GeneratedFile, field *protogen.Field) (goType string, pointer bool) {
	if field.Desc.IsWeak() {
		return "struct{}", false
	}

	pointer = field.Desc.HasPresence()
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		goType = "bool"
	case protoreflect.EnumKind:
		goType = g.QualifiedGoIdent(field.Enum.GoIdent)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		goType = "int32"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		goType = "uint32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		goType = "int64"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		goType = "uint64"
	case protoreflect.FloatKind:
		goType = "float32"
	case protoreflect.DoubleKind:
		goType = "float64"
	case protoreflect.StringKind:
		goType = "string"
	case protoreflect.BytesKind:
		goType = "[]byte"
		pointer = false // rely on nullability of slices for presence
	case protoreflect.MessageKind, protoreflect.GroupKind:
		goType = "*" + g.QualifiedGoIdent(field.Message.GoIdent)
		pointer = false // pointer captured as part of the type
	}
	switch {
	case field.Desc.IsList():
		return "[]" + goType, false
	case field.Desc.IsMap():
		keyType, _ := fieldGoType(g, field.Message.Fields[0])
		valType, _ := fieldGoType(g, field.Message.Fields[1])
		return fmt.Sprintf("map[%v]%v", keyType, valType), false
	}
	return goType, pointer
}

func parseType(str string) (goType string, pointer bool) {
	if len(str) > 0 && str[:1] == "*" {
		return str[1:], true
	}
	return str, false
}

func toLowerFirst(str string) string {
	return strings.ToLower(str[:1]) + str[1:]
}

func getPackageName() string {
	mod, _ := ioutil.ReadFile("go.mod")
	return modfile.ModulePath(mod)
}

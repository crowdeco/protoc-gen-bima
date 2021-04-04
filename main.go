package main

import (
	"flag"

	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	var (
		flags flag.FlagSet
		// importPrefix = flags.String("import_prefix", "", "prefix to prepend to import paths")
	)
	importRewriteFunc := func(importPath protogen.GoImportPath) protogen.GoImportPath {
		switch importPath {
		case "context", "fmt", "math":
			return importPath
		}
		// if *importPrefix != "" {
		// 	return protogen.GoImportPath(*importPrefix) + importPath
		// }
		return importPath
	}
	protogen.Options{
		ParamFunc:         flags.Set,
		ImportRewriteFunc: importRewriteFunc,
	}.Run(func(gen *protogen.Plugin) error {
		BimaPlugin{}.Generate(gen)
		return nil
	})
}

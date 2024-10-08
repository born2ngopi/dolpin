package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/born2ngopi/dolphin/prompt"
)

type Config struct {
	Path          string
	FuncName      string
	ModulePath    string
	Dir           string
	MockLib       string
	MockDir       string
	ExistingTests map[string]string
}

// getListFunctionName is a function to get all function name from a file
// that start with "Test" prefix
// example:
// func TestHello(t *testing.T) {}
// func TestWorld(t *testing.T) {}
// then this function will return
// []string{"TestHello", "TestWorld"}
func getListFunctionName(path string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	var funcNames []string

	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {

			if strings.HasPrefix(funcDecl.Name.Name, "Test") {
				funcNames = append(funcNames, funcDecl.Name.Name)
			}
		}
	}

	return funcNames
}

func readFileToPrompt(conf Config) (promptResult prompt.Template, packageName string, err error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, conf.Path, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	packageName = file.Name.Name

	// get source code
	sourceCode := getSourceCode(file.Pos(), file.End(), fset)
	if sourceCode != "" {
		promptResult.SourceCode = sourceCode
	}

	// prepare import path
	importPath := make(map[string]string)

	// read all import path
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			if genDecl.Tok == token.IMPORT {
				for _, spec := range genDecl.Specs {
					if importSpec, ok := spec.(*ast.ImportSpec); ok {
						if importSpec.Name != nil {
							importPath[importSpec.Name.Name] = importSpec.Path.Value
						} else {
							_importPath := strings.ReplaceAll(filepath.Base(importSpec.Path.Value), "\"", "")
							importPath[_importPath] = importSpec.Path.Value
						}
					}
				}
			}
		}
	}

	var isAnyFunc, skipAll bool
	// check function'
fileLoop:
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			isAnyFunc = true

			if conf.FuncName != "" && conf.FuncName != funcDecl.Name.Name {
				continue
			}

			if conf.FuncName == "" && conf.ExistingTests != nil {
				// if funcName not set,
				// it's posibility from walk dir
				// we wan check if function is already have unit test or not
				for name, path := range conf.ExistingTests {
					if strings.Contains(name, funcDecl.Name.Name) {
						if skipAll {
							continue fileLoop
						}
						multiSpinner.Stop()
						fmt.Printf(
							"Function %s already have unit test on %s,\ndo you still want to generate? [y/N]\nor press [s] for skip all\n",
							funcDecl.Name.Name,
							path,
						)
						var stillGen string
						fmt.Scanln(&stillGen)

						if stillGen == "y" || stillGen == "Y" {
							multiSpinner.Start("Generate code completion....")
							break
						} else if stillGen == "s" || stillGen == "S" {
							skipAll = true
							continue fileLoop
						} else {
							continue fileLoop
						}
					}
				}
			}

			if funcDecl.Type.Params != nil {
				for _, field := range funcDecl.Type.Params.List {
					// check if type variable is struct
					if field.Type != nil {
						if selExp, ok := field.Type.(*ast.SelectorExpr); ok {

							// check if struct is from import
							if ident, ok := selExp.X.(*ast.Ident); ok {
								if importPath[ident.Name] != "" {

									// adding gopath with import path
									gopath := os.Getenv("GOPATH")

									pathDir := strings.ReplaceAll(importPath[ident.Name], "\"", "")
									pathDir = gopath + "/src/" + pathDir

									structs, err := getStructFromImportPackage(pathDir, importPath[ident.Name], selExp)
									if err != nil {
										return promptResult, packageName, err
									}

									promptResult.Structs = append(promptResult.Structs, structs...)

								}
							}
						} else if selExp, ok := field.Type.(*ast.Ident); ok {
							// check if struct is from same file or sampe package
							if _struct, ok := Struct[packageName+selExp.Name]; ok {
								_struct.From = "same package"
								promptResult.Structs = append(promptResult.Structs, _struct)
							}
						}
					}
				}
			}

			promptResult.Mock = prompt.Mock{
				Name: conf.MockLib,
				Dir:  conf.MockDir,
			}

		}
	}

	if !isAnyFunc {
		promptResult.SourceCode = ""
	}

	return promptResult, packageName, nil
}

func getStructFromImportPackage(pathDir string, importPath string, selExp *ast.SelectorExpr) ([]prompt.Struct, error) {

	var structs []prompt.Struct

	err := filepath.Walk(pathDir, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		importFile, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		structName := selExp.Sel.Name

		if _struct, ok := Struct[structName]; ok {
			_struct.From = importPath
			structs = append(structs, _struct)
		} else {

			structFieldMap := findStructFields(importFile, structName)

			_struct := prompt.Struct{
				Name: structName,
			}
			if len(structFieldMap) > 0 {
				for fieldName, fieldType := range structFieldMap {
					_struct.Fields = append(_struct.Fields, prompt.StructField{
						Name: fieldName,
						Type: fieldType,
					})
				}
			}

			Struct[structName] = _struct
			_struct.From = importPath
			structs = append(structs, _struct)
		}

		return nil
	})

	return structs, err
}

// getStructFromStatement is a function to get struct declaration from statement
// example:
// var user = user.User{}
// and User have field
//   - Name string
//   - Age int
//
// then this function will return
//
//	[]prompt.Struct{
//	   {
//	      Name: "User",
//	      Fields: []prompt.Field{
//	         {
//	            Name: "Name",
//	            Type: "string",
//	         },
//	         {
//	            Name: "Age",
//	            Type: "int",
//	         },
//	      },
//	   },
func getStructFromStatement(decl ast.Decl, importPath map[string]string) ([]prompt.Struct, error) {
	var structs []prompt.Struct

	if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
		for _, spec := range genDecl.Specs {
			if valueSpec, ok := spec.(*ast.ValueSpec); ok {
				for range valueSpec.Names {
					// check if type variable is struct
					if valueSpec.Type != nil {
						if selExp, ok := valueSpec.Type.(*ast.SelectorExpr); ok {
							// check if struct is from import
							if ident, ok := selExp.X.(*ast.Ident); ok {
								if importPath[ident.Name] != "" {

									err := filepath.Walk(importPath[ident.Name], func(path string, info os.FileInfo, _ error) error {
										// check is file with .go extension
										if info.IsDir() || filepath.Ext(path) != ".go" {
											return nil
										}

										importFile, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
										if err != nil {
											return err
										}

										structName := selExp.Sel.Name

										if _struct, ok := Struct[structName]; ok {
											structs = append(structs, _struct)
										} else {

											structFieldMap := findStructFields(importFile, structName)

											_struct := prompt.Struct{
												Name: structName,
											}
											if len(structFieldMap) > 0 {
												for fieldName, fieldType := range structFieldMap {
													_struct.Fields = append(_struct.Fields, prompt.StructField{
														Name: fieldName,
														Type: fieldType,
													})
												}
											}

											Struct[structName] = _struct
											structs = append(structs, _struct)
										}

										return nil
									})
									if err != nil {
										return nil, err
									}

								}
							} else {
								// read field struct
								// TODO: check if struct is from same file or sampe package
							}
						}
					}
				}
			}
		}
	}

	return structs, nil
}

func findStructFields(file *ast.File, structName string) map[string]string {
	structFieldMap := make(map[string]string)

	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == structName {

						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								for _, name := range field.Names {
									fieldName := name.Name
									fieldType := getTypeString(field.Type)
									structFieldMap[fieldName] = fieldType
								}
							}
						}
					}
				}
			}
		}
	}

	return structFieldMap
}

func getTypeString(expr ast.Expr) string {
	return types.ExprString(expr)
}

func getSourceCode(start, end token.Pos, fset *token.FileSet) string {
	startOffset := fset.Position(start).Offset
	endOffset := fset.Position(end).Offset

	file, err := os.ReadFile(fset.Position(start).Filename)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	return string(file)[startOffset:endOffset]
}

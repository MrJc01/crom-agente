package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// GoField representa um campo de struct Go parseado
type GoField struct {
	Name        string
	Type        string
	JSONTag     string
	IsOmitEmpty bool
	Comment     string
}

// GoStruct representa uma struct Go parseada
type GoStruct struct {
	Name    string
	Fields  []GoField
	Comment string
}

// ExprToString converte uma expressão AST de tipo para string
func ExprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + ExprToString(t.X)
	case *ast.ArrayType:
		return "[]" + ExprToString(t.Elt)
	case *ast.MapType:
		return "map[" + ExprToString(t.Key) + "]" + ExprToString(t.Value)
	case *ast.SelectorExpr:
		return ExprToString(t.X) + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "any"
	}
}

// ParseGoStructs faz o parsing AST de um arquivo Go e extrai structs com seus campos
func ParseGoStructs(filePath string) ([]GoStruct, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var structs []GoStruct

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			gs := GoStruct{
				Name: typeSpec.Name.Name,
			}

			if genDecl.Doc != nil {
				gs.Comment = genDecl.Doc.Text()
			}

			for _, field := range structType.Fields.List {
				gf := GoField{}

				if len(field.Names) > 0 {
					gf.Name = field.Names[0].Name
				}

				gf.Type = ExprToString(field.Type)

				if field.Tag != nil {
					tag := field.Tag.Value
					gf.JSONTag, gf.IsOmitEmpty = extractJSONTag(tag)
				}

				if field.Comment != nil {
					gf.Comment = strings.TrimSpace(field.Comment.Text())
				}

				gs.Fields = append(gs.Fields, gf)
			}

			structs = append(structs, gs)
		}
	}

	return structs, nil
}

// extractJSONTag extrai o valor da tag json de uma tag completa de struct e indica se tem omitempty
func extractJSONTag(tag string) (string, bool) {
	tag = strings.Trim(tag, "`")
	for _, part := range strings.Split(tag, " ") {
		if strings.HasPrefix(part, `json:"`) {
			val := strings.TrimPrefix(part, `json:"`)
			val = strings.TrimSuffix(val, `"`)
			parts := strings.Split(val, ",")
			hasOmitEmpty := false
			for _, p := range parts[1:] {
				if p == "omitempty" {
					hasOmitEmpty = true
					break
				}
			}
			return parts[0], hasOmitEmpty
		}
	}
	return "", false
}

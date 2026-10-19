// Package resetgen реализует генератор кода: он сканирует Go-пакеты
// в поисках структур, помеченных комментарием `// generate:reset`,
// и создаёт для каждой из них метод Reset() в файле reset.gen.go
// внутри того же пакета.
//
// Ключевая особенность: для разрешения именованных типов (например,
// `type UserID string`) используется go/types. Это позволяет корректно
// сбрасывать поля, чей статический тип — именованный алиас над
// примитивом или слайсом/мапой.
package resetgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ResetComment — комментарий-маркер, помечающий структуру для генерации.
	ResetComment = "generate:reset"
	// GeneratedFile — имя файла, который создаёт генератор.
	GeneratedFile = "reset.gen.go"
)

// Run обходит дерево каталогов, начиная с root, и (пере)создаёт
// reset.gen.go для каждого Go-пакета, в котором есть хотя бы одна
// структура, помеченная маркером.
func Run(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == "vendor" || name == "testdata" {
			return filepath.SkipDir
		}
		if name != "." && name != ".." && strings.HasPrefix(name, ".") {
			return filepath.SkipDir
		}
		return processDir(path)
	})
}

// processDir парсит все .go-файлы одного каталога, типизирует их через
// go/types и, если найдены структуры с маркером, перезаписывает reset.gen.go.
func processDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	filesByPkg := make(map[string][]*ast.File)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") ||
			name == GeneratedFile {
			continue
		}
		fullPath := filepath.Join(dir, name)
		f, parseErr := parser.ParseFile(fset, fullPath, nil, parser.ParseComments)
		if parseErr != nil {
			continue
		}
		filesByPkg[f.Name.Name] = append(filesByPkg[f.Name.Name], f)
	}

	for pkgName, files := range filesByPkg {
		// Типизируем пакет. Ошибки намеренно игнорируем: нам нужно
		// лишь то, что удалось разрешить. Например, если у пользователя
		// временно не резолвится импорт, генератор всё равно отработает
		// по тем полям, типы которых уже известны.
		info := &types.Info{
			Types: make(map[ast.Expr]types.TypeAndValue),
			Uses:  make(map[*ast.Ident]types.Object),
			Defs:  make(map[*ast.Ident]types.Object),
		}
		conf := types.Config{
			Importer: importer.ForCompiler(fset, "source", nil),
			Error:    func(error) {},
		}
		_, _ = conf.Check(pkgName, fset, files, info)

		structs := findStructs(files)
		if len(structs) == 0 {
			continue
		}
		code, genErr := generate(pkgName, structs, info)
		if genErr != nil {
			return fmt.Errorf("%s: %w", dir, genErr)
		}
		out := filepath.Join(dir, GeneratedFile)
		if writeErr := os.WriteFile(out, code, 0o644); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// structInfo хранит метаданные, необходимые для генерации метода Reset.
type structInfo struct {
	name   string
	fields []fieldInfo
}

// fieldInfo описывает одно поле структуры: имя и его AST-тип.
type fieldInfo struct {
	name string
	expr ast.Expr
}

// findStructs возвращает все структуры, объявление типа которых
// помечено маркером `// generate:reset`.
func findStructs(files []*ast.File) []*structInfo {
	var result []*structInfo
	for _, file := range files {
		for _, decl := range file.Decls {
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
				if !hasResetComment(genDecl.Doc) && !hasResetComment(typeSpec.Doc) {
					continue
				}
				info := &structInfo{name: typeSpec.Name.Name}
				for _, field := range structType.Fields.List {
					if len(field.Names) == 0 {
						continue
					}
					for _, n := range field.Names {
						info.fields = append(info.fields, fieldInfo{
							name: n.Name,
							expr: field.Type,
						})
					}
				}
				result = append(result, info)
			}
		}
	}
	return result
}

// hasResetComment проверяет, содержит ли группа комментариев
// маркер `generate:reset`.
func hasResetComment(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		text := strings.TrimSpace(c.Text)
		text = strings.TrimSpace(strings.TrimPrefix(text, "//"))
		text = strings.TrimSpace(strings.TrimPrefix(text, "/*"))
		text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
		if text == ResetComment {
			return true
		}
	}
	return false
}

// generate формирует исходник Go-файла с методами Reset для всех структур.
func generate(pkgName string, structs []*structInfo, info *types.Info) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by cmd/reset. DO NOT EDIT.\n\n")
	fmt.Fprintf(&buf, "package %s\n\n", pkgName)

	for _, s := range structs {
		fmt.Fprintf(&buf, "func (rs *%s) Reset() {\n", s.name)
		buf.WriteString("\tif rs == nil {\n\t\treturn\n\t}\n")
		for _, f := range s.fields {
			code := genReset("rs."+f.name, f.expr, info)
			if code == "" {
				continue
			}
			buf.WriteString(indent(code, 1))
			buf.WriteString("\n")
		}
		buf.WriteString("}\n\n")
	}

	src := buf.Bytes()
	formatted, err := format.Source(src)
	if err != nil {
		return src, fmt.Errorf("format: %w", err)
	}
	return formatted, nil
}

// resolveType достаёт types.Type для ast.Expr через go/types.
// Если info == nil или тип не найден, возвращает nil — вызывающая
// сторона тогда использует AST-fallback.
func resolveType(expr ast.Expr, info *types.Info) types.Type {
	if info == nil || expr == nil {
		return nil
	}
	// Основной путь: go/types уже записал тип для выражения.
	if tv, ok := info.Types[expr]; ok && tv.Type != nil && tv.Type != types.Typ[types.Invalid] {
		return tv.Type
	}
	// Fallback на AST-обход для случаев, когда выражение — идентификатор.
	switch t := expr.(type) {
	case *ast.Ident:
		if obj, ok := info.Uses[t]; ok && obj != nil {
			return obj.Type()
		}
		if obj := types.Universe.Lookup(t.Name); obj != nil {
			return obj.Type()
		}
	case *ast.StarExpr:
		if inner := resolveType(t.X, info); inner != nil {
			return types.NewPointer(inner)
		}
	case *ast.ArrayType:
		if t.Len == nil {
			if elem := resolveType(t.Elt, info); elem != nil {
				return types.NewSlice(elem)
			}
		}
	case *ast.MapType:
		k := resolveType(t.Key, info)
		v := resolveType(t.Value, info)
		if k != nil && v != nil {
			return types.NewMap(k, v)
		}
	case *ast.SelectorExpr:
		if obj, ok := info.Uses[t.Sel]; ok && obj != nil {
			return obj.Type()
		}
	}
	return nil
}

// genReset возвращает код, сбрасывающий target. Если тип удаётся
// разрешить через go/types — используется genResetType, иначе
// откатываемся на чисто синтаксический genResetAST.
func genReset(target string, expr ast.Expr, info *types.Info) string {
	if t := resolveType(expr, info); t != nil {
		return genResetType(target, t)
	}
	return genResetAST(target, expr)
}

// genResetType генерирует сброс значения target с учётом underlying-типа.
func genResetType(target string, t types.Type) string {
	if t == nil {
		return ""
	}
	underlying := t.Underlying()

	// Примитивы (в том числе именованные: type UserID string, type Flags uint8).
	if basic, ok := underlying.(*types.Basic); ok {
		// Если у именованного типа есть собственный Reset() —
		// уважаем его и зовём через интерфейс.
		if _, isNamed := t.(*types.Named); isNamed && hasResetMethod(t) {
			return tryResetCall(target)
		}
		return genZeroBasic(target, basic)
	}

	// Структуры (в том числе именованные): пробуем интерфейсный Reset().
	// Для структур без метода это безопасный no-op.
	if _, ok := underlying.(*types.Struct); ok {
		return tryResetCall(target)
	}

	switch u := underlying.(type) {
	case *types.Slice:
		return target + " = " + target + "[:0]"
	case *types.Map:
		return "clear(" + target + ")"
	case *types.Pointer:
		return genResetPointerType(target, u.Elem())
	case *types.Array:
		inner := genResetType(target+"[i]", u.Elem())
		if inner == "" {
			return ""
		}
		return fmt.Sprintf("for i := range %s {\n%s\n}", target, indent(inner, 1))
	case *types.Interface, *types.Chan, *types.Signature:
		return target + " = nil"
	}
	return ""
}

// genResetPointerType обрабатывает указатель: разыменовывает и сбрасывает.
func genResetPointerType(target string, elem types.Type) string {
	if elem == nil {
		return ""
	}
	// Указатель на именованную структуру (или что-то с Reset()) —
	// вызываем Reset() прямо на указателе, без разыменования.
	if _, isStruct := elem.Underlying().(*types.Struct); isStruct || hasResetMethod(elem) {
		return fmt.Sprintf(
			"if resetter, ok := any(%s).(interface{ Reset() }); ok && %s != nil {\n\tresetter.Reset()\n}",
			target, target,
		)
	}
	// Иначе — разыменовываем и сбрасываем по underlying-правилам.
	if basic, ok := elem.Underlying().(*types.Basic); ok {
		code := genZeroBasic("(*"+target+")", basic)
		return fmt.Sprintf("if %s != nil {\n%s\n}", target, indent(code, 1))
	}
	code := genResetType("(*"+target+")", elem)
	if code == "" {
		return ""
	}
	return fmt.Sprintf("if %s != nil {\n%s\n}", target, indent(code, 1))
}

// tryResetCall генерирует вызов Reset() через интерфейсный type assertion.
func tryResetCall(target string) string {
	return fmt.Sprintf(
		"if resetter, ok := any(&%s).(interface{ Reset() }); ok {\n\tresetter.Reset()\n}",
		target,
	)
}

// hasResetMethod проверяет наличие метода Reset() у типа t
// (с учётом pointer-приёмника).
func hasResetMethod(t types.Type) bool {
	if t == nil {
		return false
	}
	if _, ok := t.Underlying().(*types.Interface); ok {
		return false
	}
	if types.NewMethodSet(t).Lookup(nil, "Reset") != nil {
		return true
	}
	if types.NewMethodSet(types.NewPointer(t)).Lookup(nil, "Reset") != nil {
		return true
	}
	return false
}

// genZeroBasic генерирует присваивание нулевого значения для базового типа.
func genZeroBasic(target string, b *types.Basic) string {
	switch {
	case b.Info()&types.IsString != 0:
		return target + ` = ""`
	case b.Info()&types.IsBoolean != 0:
		return target + " = false"
	case b.Info()&(types.IsInteger|types.IsUnsigned|types.IsFloat|types.IsComplex) != 0:
		return target + " = 0"
	case b.Kind() == types.UnsafePointer:
		return target + " = nil"
	}
	return ""
}

// genResetAST — синтаксический fallback на случай, когда типизация
// полностью провалилась (info == nil). Повторяет поведение до внедрения
// go/types.
func genResetAST(target string, expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
			"float32", "float64", "complex64", "complex128",
			"byte", "rune":
			return target + " = 0"
		case "string":
			return target + ` = ""`
		case "bool":
			return target + " = false"
		case "error", "any":
			return target + " = nil"
		}
		return tryResetCall(target)
	case *ast.StarExpr:
		switch inner := t.X.(type) {
		case *ast.Ident:
			if isASTPrimitive(inner.Name) || inner.Name == "error" || inner.Name == "any" {
				return fmt.Sprintf(
					"if %s != nil {\n%s\n}",
					target,
					indent(genResetAST("(*"+target+")", inner), 1),
				)
			}
			return fmt.Sprintf(
				"if resetter, ok := any(%s).(interface{ Reset() }); ok && %s != nil {\n\tresetter.Reset()\n}",
				target, target,
			)
		}
		code := genResetAST("(*"+target+")", t.X)
		if code == "" {
			return ""
		}
		return fmt.Sprintf("if %s != nil {\n%s\n}", target, indent(code, 1))
	case *ast.ArrayType:
		if t.Len == nil {
			return target + " = " + target + "[:0]"
		}
		inner := genResetAST(target+"[i]", t.Elt)
		if inner == "" {
			return ""
		}
		return fmt.Sprintf("for i := range %s {\n%s\n}", target, indent(inner, 1))
	case *ast.MapType:
		return fmt.Sprintf("clear(%s)", target)
	case *ast.InterfaceType, *ast.ChanType, *ast.FuncType:
		return target + " = nil"
	case *ast.SelectorExpr:
		return tryResetCall(target)
	}
	return ""
}

// isASTPrimitive — синтаксическая проверка имени примитивного типа.
func isASTPrimitive(name string) bool {
	switch name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128",
		"byte", "rune", "string", "bool":
		return true
	}
	return false
}

// indent добавляет к каждой непустой строке s префикс из level табов.
func indent(s string, level int) string {
	prefix := strings.Repeat("\t", level)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

package resetgen

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- вспомогательные функции ----------------------------------------------

// writeFile создаёт файл по указанному пути (создавая директории).
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mustParse парсит исходник и возвращает AST-файл.
func mustParse(t *testing.T, name, src string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func mustParseWithFset(t *testing.T, name, src string) (*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return f, fset
}

// parseExpr парсит выражение типа (например, "[]int").
func parseExpr(t *testing.T, s string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// typeOf вычисляет types.Type выражения expr в пакете, определяемом decls.
// Парсинг и Check используют один и тот же FileSet — обязательное условие.
func typeOf(t *testing.T, expr string, decls ...string) types.Type {
	t.Helper()
	src := "package p\n" + strings.Join(decls, "\n")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Uses:  make(map[*ast.Ident]types.Object),
		Defs:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Error: func(error) {}}
	pkg, _ := conf.Check("p", fset, []*ast.File{f}, info)
	tv, err := types.Eval(fset, pkg, f.Pos(), expr)
	if err != nil {
		t.Fatalf("types.Eval(%q): %v", expr, err)
	}
	return tv.Type
}

// typeCheckInfo типизирует уже распарсенный файл и возвращает types.Info.
func typeCheckInfo(t *testing.T, fset *token.FileSet, f *ast.File) *types.Info {
	t.Helper()
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Uses:  make(map[*ast.Ident]types.Object),
		Defs:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Error: func(error) {}}
	_, _ = conf.Check(f.Name.Name, fset, []*ast.File{f}, info)
	return info
}

// --- hasResetComment -------------------------------------------------------

func TestHasResetComment(t *testing.T) {
	cases := []struct {
		name string
		doc  *ast.CommentGroup
		want bool
	}{
		{"nil", nil, false},
		{"точный", docGroup("// generate:reset"), true},
		{"с_пробелами", docGroup("//    generate:reset   "), true},
		{"другой_маркер", docGroup("// something:else"), false},
		{"в_группе", docGroup("// Some doc\n// generate:reset"), true},
		{"префикс_только", docGroup("// generate:reset-extra"), false},
		{"блочный_комментарий", docGroup("/* generate:reset */"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasResetComment(c.doc); got != c.want {
				t.Fatalf("hasResetComment() = %v, ожидалось %v", got, c.want)
			}
		})
	}
}

// docGroup создаёт группу комментариев из текста, парся минимальный файл.
func docGroup(text string) *ast.CommentGroup {
	src := "package p\n" + text + "\ntype T struct{}\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	// Комментарий может быть привязан либо к GenDecl (type), либо к TypeSpec.
	decl, ok := f.Decls[0].(*ast.GenDecl)
	if !ok {
		panic("docGroup: ожидался *ast.GenDecl")
	}
	if decl.Doc != nil {
		return decl.Doc
	}
	spec, ok := decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		panic("docGroup: ожидался *ast.TypeSpec")
	}
	return spec.Doc
}

// --- genResetType ----------------------------------------------------------

func TestGenResetType(t *testing.T) {
	cases := []struct {
		name  string
		expr  string
		decls []string
		wants []string
	}{
		{"int", "int", nil, []string{"x = 0"}},
		{"int64", "int64", nil, []string{"x = 0"}},
		{"string", "string", nil, []string{`x = ""`}},
		{"bool", "bool", nil, []string{"x = false"}},
		{"slice", "[]int", nil, []string{"x = x[:0]"}},
		{"map", "map[string]int", nil, []string{"clear(x)"}},
		{"chan", "chan int", nil, []string{"x = nil"}},
		{"func", "func()", nil, []string{"x = nil"}},
		{"any", "any", nil, []string{"x = nil"}},

		// Именованные типы над примитивами.
		{
			"named_string", "UserID",
			[]string{"type UserID string"},
			[]string{`x = ""`},
		},
		{
			"named_uint8", "Flags",
			[]string{"type Flags uint8"},
			[]string{"x = 0"},
		},
		{
			"named_int64", "Counter",
			[]string{"type Counter int64"},
			[]string{"x = 0"},
		},
		{
			"named_bool", "Enabled",
			[]string{"type Enabled bool"},
			[]string{"x = false"},
		},

		// Именованные slice/map.
		{
			"named_slice", "Items",
			[]string{"type Items []int"},
			[]string{"x = x[:0]"},
		},
		{
			"named_map", "Lookup",
			[]string{"type Lookup map[string]int"},
			[]string{"clear(x)"},
		},

		// Структуры — через интерфейсный Reset().
		{
			"named_struct", "MyStruct",
			[]string{"type MyStruct struct{}"},
			[]string{"any(&x).(interface{ Reset() })", "resetter.Reset()"},
		},

		// Указатели.
		{
			"ptr_to_string", "*string", nil,
			[]string{"x != nil", `(*x) = ""`},
		},
		{
			"ptr_to_named_string", "*UserID",
			[]string{"type UserID string"},
			[]string{"x != nil", `(*x) = ""`},
		},
		{
			"ptr_to_slice", "*[]int", nil,
			[]string{"x != nil", "(*x) = (*x)[:0]"},
		},
		{
			"ptr_to_named", "*MyStruct",
			[]string{"type MyStruct struct{}"},
			[]string{"any(x).(interface{ Reset() })", "x != nil", "resetter.Reset()"},
		},

		// Массивы.
		{
			"fixed_array", "[4]int", nil,
			[]string{"for i := range x {", "x[i] = 0"},
		},
		{
			"fixed_array_named", "[4]UserID",
			[]string{"type UserID string"},
			[]string{"for i := range x {", `x[i] = ""`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := genResetType("x", typeOf(t, c.expr, c.decls...))
			for _, w := range c.wants {
				if !strings.Contains(got, w) {
					t.Fatalf("genResetType(%q) = %q\nне найдено %q", c.expr, got, w)
				}
			}
		})
	}
}

// --- findStructs -----------------------------------------------------------

func TestFindStructs(t *testing.T) {
	src := `package p

// generate:reset
type A struct {
	N int
	S string
}

// Не помечена.
type B struct{ N int }

// generate:reset
type C struct {
	X, Y int
	Embedded
}
`
	f := mustParse(t, "p.go", src)

	structs := findStructs([]*ast.File{f})
	if len(structs) != 2 {
		t.Fatalf("ожидалось 2 структуры, получено %d (%v)", len(structs), names(structs))
	}
	if structs[0].name != "A" || structs[1].name != "C" {
		t.Fatalf("неожиданные имена структур: %v", names(structs))
	}
	if len(structs[0].fields) != 2 {
		t.Fatalf("A: ожидалось 2 поля, получено %d", len(structs[0].fields))
	}
	// Встроенное поле пропускается, X и Y дают две записи.
	if len(structs[1].fields) != 2 {
		t.Fatalf("C: ожидалось 2 поля, получено %d", len(structs[1].fields))
	}
}

// names извлекает имена структур для сообщений об ошибках.
func names(s []*structInfo) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		out = append(out, v.name)
	}
	return out
}

// --- generate --------------------------------------------------------------

func TestGenerate_OutputIsValidGo(t *testing.T) {
	// ВАЖНО: парсим и типизируем одним и тем же FileSet'ом,
	// иначе go/types падает с SIGSEGV в (*Checker).filename.
	f, fset := mustParseWithFset(t, "p.go", `package p

// generate:reset
type S struct {
	N int
	P *string
	L []int
	M map[string]int
}

type UserID string

// generate:reset
type WithNamed struct {
	ID UserID
}
`)
	info := typeCheckInfo(t, fset, f)
	structs := findStructs([]*ast.File{f})

	code, err := generate("p", structs, info)
	if err != nil {
		t.Fatal(err)
	}

	// Для проверки синтаксиса сгенерированного файла — отдельный FileSet,
	// это безопасно, потому что здесь не вызывается types.Check.
	outFset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(outFset, "reset.gen.go", code, 0); parseErr != nil {
		t.Fatalf("сгенерированный код — не валидный Go: %v\n%s", parseErr, code)
	}
	for _, want := range []string{
		"func (rs *S) Reset()",
		"rs.N = 0",
		`(*rs.P) = ""`,
		"rs.L = rs.L[:0]",
		"clear(rs.M)",
		// Именованный тип над примитивом корректно сбрасывается.
		`rs.ID = ""`,
	} {
		if !strings.Contains(string(code), want) {
			t.Fatalf("ожидалось %q в:\n%s", want, code)
		}
	}
}

// --- Run (интеграционные тесты) --------------------------------------------

func TestRun_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sample.go"), `package sample

type UserID string

// generate:reset
type Sample struct {
	N     int
	S     string
	Items []int
	M     map[string]int
	P     *string
	Child *Sample
	ID    UserID
}
`)
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, GeneratedFile)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("сгенерированный файл отсутствует: %v", err)
	}

	// Проверяем, что это валидный Go (без types.Check — отдельный FileSet ок).
	fset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(fset, out, data, 0); parseErr != nil {
		t.Fatalf("сгенерированный файл — не валидный Go: %v\n%s", parseErr, data)
	}

	for _, want := range []string{
		"package sample",
		"func (rs *Sample) Reset()",
		"rs.N = 0",
		`rs.S = ""`,
		"rs.Items = rs.Items[:0]",
		"clear(rs.M)",
		`(*rs.P) = ""`,
		"resetter.Reset()",
		// Ключевое: именованный string сброшен.
		`rs.ID = ""`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("ожидалось %q в сгенерированном файле:\n%s", want, data)
		}
	}
}

func TestRun_MultipleStructsInOnePackage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "p.go"), `package p

// generate:reset
type A struct{ N int }

// generate:reset
type B struct{ S string }

type C struct{ N int }
`)
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, GeneratedFile))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "func (rs *A) Reset()") {
		t.Fatal("A отсутствует")
	}
	if !strings.Contains(s, "func (rs *B) Reset()") {
		t.Fatal("B отсутствует")
	}
	if strings.Contains(s, "func (rs *C) Reset()") {
		t.Fatal("C не должна генерироваться")
	}
}

func TestRun_MultiplePackages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "root.go"), `package root
// generate:reset
type R struct{ N int }
`)
	sub := filepath.Join(dir, "sub")
	writeFile(t, filepath.Join(sub, "s.go"), `package sub
// generate:reset
type S struct{ N int }
`)
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, GeneratedFile)); err != nil {
		t.Fatalf("root reset.gen.go отсутствует: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, GeneratedFile)); err != nil {
		t.Fatalf("sub reset.gen.go отсутствует: %v", err)
	}
}

func TestRun_SkipsVendorTestdataHidden(t *testing.T) {
	dir := t.TempDir()
	skipped := []string{"vendor/v", "testdata/t", ".git/g"}
	for _, sub := range skipped {
		writeFile(t, filepath.Join(dir, sub, "x.go"), `package x
// generate:reset
type X struct{ N int }
`)
	}
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}
	for _, sub := range skipped {
		out := filepath.Join(dir, sub, GeneratedFile)
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("ожидалось, что %s будет пропущен, но он создан", out)
		}
	}
}

func TestRun_IgnoresTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x_test.go"), `package x
// generate:reset
type X struct{ N int }
`)
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, GeneratedFile)); !os.IsNotExist(err) {
		t.Fatal("reset.gen.go не должен создаваться на основе только _test.go")
	}
}

func TestRun_Idempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.go"), `package x
// generate:reset
type X struct{ N int }
`)
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}
	// Внешняя переменная называется readErr, чтобы не затенять err
	// из if-блока ниже (govet shadow).
	first, readErr := os.ReadFile(filepath.Join(dir, GeneratedFile))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}
	second, readErr := os.ReadFile(filepath.Join(dir, GeneratedFile))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("повторный запуск дал другой результат:\n--- первый ---\n%s\n--- второй ---\n%s", first, second)
	}
}

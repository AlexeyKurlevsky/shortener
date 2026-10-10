package resetgen

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
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

// parseExpr парсит выражение типа (например, "[]int").
func parseExpr(t *testing.T, s string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(s)
	if err != nil {
		t.Fatal(err)
	}
	return e
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

// --- genReset --------------------------------------------------------------

func TestGenReset(t *testing.T) {
	cases := []struct {
		name  string
		expr  string
		wants []string
	}{
		{"int", "int", []string{"x = 0"}},
		{"int64", "int64", []string{"x = 0"}},
		{"string", "string", []string{`x = ""`}},
		{"bool", "bool", []string{"x = false"}},
		{"slice", "[]int", []string{"x = x[:0]"}},
		{"map", "map[string]int", []string{"clear(x)"}},
		{"chan", "chan int", []string{"x = nil"}},
		{"func", "func()", []string{"x = nil"}},
		{"any", "any", []string{"x = nil"}},
		{"указатель_на_string", "*string", []string{"x != nil", `(*x) = ""`}},
		{"указатель_на_slice", "*[]int", []string{"x != nil", "(*x) = (*x)[:0]"}},
		{"массив_фикс", "[4]int", []string{"for i := range x {", "x[i] = 0"}},
		{
			"именованный_тип", "MyStruct",
			[]string{"any(&x).(interface{ Reset() })", "resetter.Reset()"},
		},
		{
			"указатель_на_именованный", "*MyStruct",
			[]string{"any(x).(interface{ Reset() })", "x != nil", "resetter.Reset()"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := genReset("x", parseExpr(t, c.expr))
			for _, w := range c.wants {
				if !strings.Contains(got, w) {
					t.Fatalf("genReset(%q) = %q\nне найдено %q", c.expr, got, w)
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
	f := mustParse(t, "p.go", `package p

// generate:reset
type S struct {
	N int
	P *string
	L []int
	M map[string]int
}
`)
	structs := findStructs([]*ast.File{f})

	code, err := generate("p", structs)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(fset, "reset.gen.go", code, 0); parseErr != nil {
		t.Fatalf("сгенерированный код — не валидный Go: %v\n%s", parseErr, code)
	}
	for _, want := range []string{
		"func (rs *S) Reset()",
		"rs.N = 0",
		`(*rs.P) = ""`,
		"rs.L = rs.L[:0]",
		"clear(rs.M)",
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

// generate:reset
type Sample struct {
	N     int
	S     string
	Items []int
	M     map[string]int
	P     *string
	Child *Sample
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

	// Проверяем, что это валидный Go.
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
	// из if-блока ниже (это ловит govet shadow).
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

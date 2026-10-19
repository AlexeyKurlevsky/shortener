// Package analyzer содержит кастомные статические анализаторы проекта.
// В частности, OsExitAnalyzer запрещает прямой вызов os.Exit в функции
// main пакета main.
package analyzer

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// OsExitAnalyzer — статический анализатор, который сообщает о прямых
// вызовах os.Exit внутри функции main пакета main.
//
// # Назначение
//
// Прямой вызов os.Exit в main обходит отложенные функции (defer),
// затрудняет тестирование и повторное использование кода. Анализатор
// требует, чтобы функция main завершалась обычным возвратом.
//
// # Использование
//
//	multichecker -osexit ./...
//
// # Реализация
//
// Обходит *ast.FuncDecl, отбирает функцию main в пакете "main" и ищет
// вызовы, разрешающиеся в объект os.Exit через pass.TypesInfo.
var OsExitAnalyzer = &analysis.Analyzer{
	Name:     "osexit",
	Doc:      "check for os.Exit calls in main function of main package",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, nil
	}

	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "main" || fn.Body == nil {
			return
		}

		// Пропускаем файлы, не принадлежащие исходникам модуля:
		// - файлы без расширения .go (артефакты кеша сборки, например ...-d);
		// - файлы, помеченные как сгенерированные в GOROOT (префикс пути
		//   содержит "/src/" в составе GOROOT невозможно отличить без
		//   runtime.GOROOT(), поэтому полагаемся на расширение и путь кеша).
		pos := pass.Fset.Position(fn.Pos())
		if !strings.HasSuffix(pos.Filename, ".go") {
			return
		}
		if strings.Contains(pos.Filename, "/go-build/") {
			return
		}

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isOsExitCall(pass, call) {
				pass.Reportf(call.Pos(),
					"direct call to os.Exit in main function is forbidden")
			}
			return true
		})
	})

	return nil, nil
}

func isOsExitCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	obj := pass.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return false
	}
	return obj.Pkg() != nil && obj.Pkg().Path() == "os" && obj.Name() == "Exit"
}

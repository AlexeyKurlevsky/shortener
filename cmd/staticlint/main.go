// Package main реализует multichecker — инструмент для комплексного
// статического анализа Go-кода. Он объединяет в себе стандартные
// анализаторы из golang.org/x/tools, анализаторы из staticcheck.io,
// сторонние публичные анализаторы и собственный анализатор OsExitAnalyzer.
//
// # Запуск
//
// Сборка:
//
//	go build -o staticlint ./cmd/staticlint
//
// Запуск анализа всех пакетов в проекте:
//
//	./staticlint ./...
//
// Запуск конкретного анализатора (например, SA1000):
//
//	./staticlint -SA1000 ./...
//
// # Состав анализаторов
//
//  1. Стандартные анализаторы пакета golang.org/x/tools/go/analysis/passes:
//     shift, printf, shadow, structtag.
//
//  2. Все анализаторы класса SA из staticcheck.io — проверки корректности
//     кода, выявляющие потенциальные ошибки.
//
//  3. Один анализатор из класса ST (stylecheck) — проверка стиля кода.
//
//  4. Публичные анализаторы:
//     - nakedret (github.com/alexkohler/nakedret) — находит naked returns
//     в функциях длиннее заданного порога.
//     - bidichk (github.com/breml/bidichk) — обнаруживает опасные
//     последовательности Unicode-символов в исходниках.
//
//  5. Собственный анализатор OsExitAnalyzer — запрещает прямой вызов
//     os.Exit в функции main пакета main.
package main

import (
	"strings"

	"github.com/AlexeyKurlevsky/shortener/internal/analyzer"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/structtag"

	"github.com/alexkohler/nakedret/v2"
	"github.com/breml/bidichk/pkg/bidichk"

	// Пакет staticcheck содержит все анализаторы класса SA.
	"honnef.co/go/tools/staticcheck"

	// Пакет stylecheck содержит анализаторы класса ST.
	"honnef.co/go/tools/stylecheck"
)

func main() {
	// Собираем все анализаторы в один слайс.
	var analyzers []*analysis.Analyzer

	// 1. Стандартные анализаторы из golang.org/x/tools/go/analysis/passes.
	analyzers = append(analyzers,
		shift.Analyzer,     // проверка сдвигов, превышающих разрядность целого
		printf.Analyzer,    // проверка соответствия format-строк и аргументов
		shadow.Analyzer,    // проверка затенения переменных
		structtag.Analyzer, // проверка корректности тегов структур
	)

	//  2. Все анализаторы класса SA из staticcheck.io.
	//     staticcheck.Analyzers — это map[int]*lint.Analyzer.
	//     Итерируемся по значениям, имя проверки берём из вложенного analysis.Analyzer.
	for _, a := range staticcheck.Analyzers {
		if strings.HasPrefix(a.Analyzer.Name, "SA") {
			analyzers = append(analyzers, a.Analyzer)
		}
	}

	//  3. Один анализатор из класса ST (stylecheck) — ST1000.
	//     Проверяет наличие комментария к пакету.
	for _, a := range stylecheck.Analyzers {
		if a.Analyzer.Name == "ST1000" {
			analyzers = append(analyzers, a.Analyzer)
			break
		}
	}

	// 4. Публичные анализаторы.
	analyzers = append(analyzers,
		nakedret.NakedReturnAnalyzer(&nakedret.NakedReturnRunner{
			MaxLength:     30,
			SkipTestFiles: true,
		}),
		bidichk.NewAnalyzer(),
	)

	// 5. Собственный анализатор.
	analyzers = append(analyzers, analyzer.OsExitAnalyzer)

	// Запускаем multichecker со всеми анализаторами.
	multichecker.Main(analyzers...)
}

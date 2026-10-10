package analyzer_test

import (
	"testing"

	"github.com/AlexeyKurlevsky/shortener/internal/analyzer"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestOsExitAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.OsExitAnalyzer, "a", "b")
}

// Command reset — точка входа CLI-утилиты генератора методов Reset.
// Вся основная логика вынесена в пакет internal/resetgen.
package main

import (
	"fmt"
	"os"

	"github.com/AlexeyKurlevsky/shortener/internal/resetgen"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := resetgen.Run(root); err != nil {
		fail(err)
	}
}

// fail печатает ошибку в stderr и завершает процесс с кодом 1.
func fail(err error) {
	fmt.Fprintln(os.Stderr, "reset generator:", err)
	os.Exit(1)
}

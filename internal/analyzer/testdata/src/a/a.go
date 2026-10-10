package main

import (
	"os"
	osAlias "os"
)

// main — функция, в которой прямой вызов os.Exit должен быть запрещён.
func main() {
	os.Exit(1) // want `direct call to os\.Exit in main function is forbidden`

	osAlias.Exit(2) // want `direct call to os\.Exit in main function is forbidden`

	// Вызовы в других функциях не являются ошибкой.
	helper()
	anotherHelper()
}

// helper — вспомогательная функция, вызов os.Exit здесь допустим.
func helper() {
	os.Exit(3) // OK: не в main
}

// anotherHelper — ещё одна вспомогательная функция.
func anotherHelper() {
	osAlias.Exit(4) // OK: не в main
}

// init также не является main, поэтому вызов допустим.
func init() {
	os.Exit(5) // OK: не main
}

package main

import (
	"flag"
)

var (
	flagRunAddr string
	flagTinyURL string
)

// parseFlags обрабатывает аргументы командной строки
// и сохраняет их значения в соответствующих переменных
func parseFlags() {
	// регистрируем переменную flagRunAddr
	// как аргумент -a со значением :8080 по умолчанию
	flag.StringVar(&flagRunAddr, "a", ":8080", "address and port to run server")
	flag.StringVar(&flagTinyURL, "b", ":8000", "port with short url")
	// парсим переданные серверу аргументы в зарегистрированные переменные
	flag.Parse()
}

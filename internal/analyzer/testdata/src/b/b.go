package a

import "os"

// NotMain — функция с именем, отличным от main, в пакете не main.
func NotMain() {
	os.Exit(1) // OK: не main и не пакет main
}

// main в пакете a (не пакет main) — тоже не должно вызывать диагностику.
func main() { // OK: пакет не main
	os.Exit(2) // OK
}

package main

import "fmt"

func classify(n int) string {
	if n < 0 {
		return "negative"
	} else if n == 0 {
		return "zero"
	} else {
		return "positive"
	}
}

func route(method string) {
	switch method {
	case "GET":
		fmt.Println("read")
	case "POST", "PUT":
		fmt.Println("write")
	default:
		fmt.Println("unsupported")
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
	}
}

func run() error { return nil }

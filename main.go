package main

import (
	"fmt"
	"net/http"
	"os"
)

type Config struct {
	Listen string `json:"listen"`
	Dial   string `json:"dial"`
}

func checkFatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
	})
	if os.Getenv("SANDSTORM") == "1" {
		sandstormMain()
	} else {
		traditionalMain()
	}
}

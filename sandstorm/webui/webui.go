package webui

import (
	"net/http"
)

func mainPage(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte(
		"Hey! IRC idler doesn't have a web interface yet, but this " +
			"placeholder page seems to be working."))
}

func ConfigureRoutes() {
	http.HandleFunc("/", mainPage)
}

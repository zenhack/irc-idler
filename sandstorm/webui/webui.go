package webui

import (
	"io"
	"net/http"
	"os"
)

const staticPath = "/opt/app/sandstorm/webui/"

// TODO: make this a template, xsrf, maybe make it not look totally bare bones.
const index = `<!DOCTYPE html>
<html>
	<head>
		<title>IRC Idler - Settings</title>
		<script src="/static/get_ip_network.js"></script>
	</head>
	<body>
		<form action="/proxy-settings" method="post">
			<div>
				<label for="host">Host:</label>
				<input type="text" id="host" name="host" />
			</div>
			<div>
				<label for="port">Port:</label>
				<input type="text" id="port" name="port" />
			</div>
			<!-- TODO: TLS checkbox -->
			<div>
				<button type="submit">Apply</button>
			</div>
		</form>
		<button id="request_cap">Request Network Access</button>
	</body>
</html>
`

func mainPage(w http.ResponseWriter, req *http.Request) {
	os.Stdout.Write([]byte("mainPage()\n"))
	if req.URL.Path != "/" {
		// The way ServeMux's routing works we can't easily specify a
		// Handler that *just* covers the root, so we do this instead.
		w.WriteHeader(404)
		w.Write([]byte("Not found"))
		return
	}
	w.Write([]byte(index))
}

func applySettings(w http.ResponseWriter, req *http.Request) {
}

func ipNetworkCap(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method not allowed.\n"))
		return
	}
	io.Copy(os.Stdout, req.Body)
}

func ConfigureRoutes() {
	http.HandleFunc("/", mainPage)
	http.Handle("/static/", http.FileServer(http.Dir(staticPath)))
	http.HandleFunc("/proxy-settings/", applySettings)
	http.HandleFunc("/ip-network-cap", ipNetworkCap)
}

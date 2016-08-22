package webui

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"golang.org/x/net/xsrftoken"
	"html/template"
	"io"
	"net/http"
	"os"
)

const staticPath = "/opt/app/sandstorm/webui/"

var (
	indexTpl = template.Must(template.ParseFiles(staticPath + "templates/index.html"))

	badXSRFToken = errors.New("Bad XSRF Token")
)

func genXSRFKey() (string, error) {
	rawBytes := make([]byte, 128/8) // 128 bit key
	_, err := rand.Read(rawBytes)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	enc := base64.NewEncoder(base64.RawStdEncoding, buf)
	enc.Write(rawBytes)
	enc.Close()
	return buf.String(), nil
}

func checkXSRF(key, userID, actionID string, req *http.Request) error {
	token := req.FormValue("_xsrf_token")
	if !xsrftoken.Valid(token, key, userID, actionID) {
		return badXSRFToken
	}
	return nil
}

func NewHandler() (http.Handler, error) {
	serveMux := http.NewServeMux()
	// TODO: might make sense to not generate this on every startup:
	xsrfKey, err := genXSRFKey()
	if err != nil {
		return nil, err
	}

	serveMux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			// The way ServeMux's routing works we can't easily specify a
			// Handler that *just* covers the root, so we do this instead.
			w.WriteHeader(404)
			w.Write([]byte("Not found"))
			return
		}
		token := xsrftoken.Generate(
			xsrfKey,
			"TODO",
			"/proxy-settings/",
		)
		tplCtx := struct{ XSRFToken string }{token}
		indexTpl.Execute(w, &tplCtx)
	})

	serveMux.Handle("/static/", http.FileServer(http.Dir(staticPath)))

	serveMux.HandleFunc("/proxy-settings/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method not allowed.\n"))
			return
		}
		if err := checkXSRF(xsrfKey, "TODO", "/proxy-settings/", req); err != nil {
			w.WriteHeader(400)
			w.Write([]byte(err.Error()))
			return
		}
		w.Write([]byte("ok!"))
	})

	serveMux.HandleFunc("/ip-network-cap", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method not allowed.\n"))
			return
		}
		io.Copy(os.Stdout, req.Body)
	})
	return serveMux, nil
}

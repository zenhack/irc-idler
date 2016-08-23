package webui

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"github.com/gorilla/mux"
	"golang.org/x/net/xsrftoken"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
)

const staticPath = "/opt/app/sandstorm/webui/"

var (
	indexTpl = template.Must(template.ParseFiles(staticPath + "templates/index.html"))

	badXSRFToken = errors.New("Bad XSRF Token")
)

type ServerConfig struct {
	Host string
	Port uint16
}

type Backend struct {
	IpNetworkCaps chan []byte
	ServerConfigs chan ServerConfig
}

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

func NewHandler(backend *Backend) (http.Handler, error) {
	r := mux.NewRouter()
	// TODO: might make sense to not generate this on every startup:
	xsrfKey, err := genXSRFKey()
	if err != nil {
		return nil, err
	}

	r.Methods("GET").Path("/").
		HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			token := xsrftoken.Generate(
				xsrfKey,
				"TODO",
				"/proxy-settings/",
			)
			tplCtx := struct{ XSRFToken string }{token}
			indexTpl.Execute(w, &tplCtx)
		})

	r.Methods("GET").PathPrefix("/static/").Handler(http.FileServer(http.Dir(staticPath)))

	r.Methods("POST").Path("/proxy-settings").
		HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if err := checkXSRF(xsrfKey, "TODO", "/proxy-settings/", req); err != nil {
				w.WriteHeader(400)
				w.Write([]byte(err.Error()))
				return
			}
			port, err := strconv.ParseUint(req.FormValue("port"), 10, 16)
			if err != nil {
				w.WriteHeader(400)
				w.Write([]byte(err.Error()))
				return
			}
			if port == 0 {
				w.WriteHeader(400)
				w.Write([]byte("Port must be non-zero"))
				return
			}
			backend.ServerConfigs <- ServerConfig{
				Host: req.FormValue("host"),
				Port: uint16(port),
			}
		})

	r.Methods("POST").Path("/ip-network-cap").
		HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Size is mostly arbitrary. This is way bigger than we
			// actually need, but it's still tiny and means we don't
			// have to think to see that it's big enough:
			limitedBody := io.LimitReader(req.Body, 512)

			dec := base64.NewDecoder(base64.RawURLEncoding, limitedBody)
			buf, err := ioutil.ReadAll(dec)
			if err != nil {
				println(err.Error())
				w.WriteHeader(400)
				return
			}
			backend.IpNetworkCaps <- buf
		})
	return r, nil
}

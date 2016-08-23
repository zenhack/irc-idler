package webui

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"golang.org/x/net/xsrftoken"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
)

const staticPath = "/opt/app/sandstorm/webui/"

var (
	indexTpl = template.Must(template.ParseFiles(staticPath + "templates/index.html"))

	badXSRFToken      = errors.New("Bad XSRF Token")
	illegalPortNumber = errors.New("Illegal Port Number (must be non-zero)")
)

type ServerConfig struct {
	Host string
	Port uint16
}

type Backend struct {
	IpNetworkCaps chan []byte
	ServerConfigs chan ServerConfig
}

type SettingsForm struct {
	Host      string `schmea:"host"`
	Port      uint16 `schema:"port"`
	XSRFToken string `schema:"_xsrf_token"`
}

func (form *SettingsForm) Validate(xsrfKey string) error {
	if !xsrftoken.Valid(form.XSRFToken, xsrfKey, "TODO", "/proxy-settings") {
		return badXSRFToken
	}
	if form.Port == 0 {
		return illegalPortNumber
	}
	return nil
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
				"/proxy-settings",
			)
			tplCtx := struct{ XSRFToken string }{token}
			indexTpl.Execute(w, &tplCtx)
		})

	r.Methods("GET").PathPrefix("/static/").Handler(http.FileServer(http.Dir(staticPath)))

	r.Methods("POST").Path("/proxy-settings").
		HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			form := &SettingsForm{}
			err := req.ParseForm()
			if err == nil {
				err = schema.NewDecoder().Decode(form, req.PostForm)
			}
			if err == nil {
				err = form.Validate(xsrfKey)
			}
			if err != nil {
				w.WriteHeader(400)
				w.Write([]byte(err.Error()))
				return
			}
			backend.ServerConfigs <- ServerConfig{
				Host: form.Host,
				Port: form.Port,
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

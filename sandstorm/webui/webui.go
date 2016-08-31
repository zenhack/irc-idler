package webui

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
	"golang.org/x/net/xsrftoken"
	"html/template"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
	"zenhack.net/go/sandstorm/capnp/grain"
	ws_capnp "zenhack.net/go/sandstorm/capnp/websession"
	"zenhack.net/go/sandstorm/websession"
	"zombiezen.com/go/capnproto2"
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

func (s *ServerConfig) String() string {
	return net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
}

type Backend struct {
	IpNetworkCaps chan capnp.Pointer
	ServerConfigs chan ServerConfig
	ClientConns   chan io.ReadWriteCloser
}

type SettingsForm struct {
	Host      string `schema:"host"`
	Port      uint16 `schema:"port"`
	XSRFToken string `schema:"_xsrf_token"`
}

type UiView struct {
	Backend *Backend
	Ctx     context.Context
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

func (v *UiView) NewSession(args grain.UiView_newSession) error {

	sessionCtx := args.Params.Context()
	serverConfig := ServerConfig{}

	r := mux.NewRouter()
	// TODO: might make sense to not generate this on every startup:
	xsrfKey, err := genXSRFKey()
	if err != nil {
		return err
	}

	r.Methods("GET").Path("/").
		HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			token := xsrftoken.Generate(
				xsrfKey,
				"TODO",
				"/proxy-settings",
			)
			indexTpl.Execute(w, &SettingsForm{
				Host:      serverConfig.Host,
				Port:      serverConfig.Port,
				XSRFToken: token,
			})
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
			serverConfig = ServerConfig{
				Host: form.Host,
				Port: form.Port,
			}
			v.Backend.ServerConfigs <- serverConfig
			http.Redirect(w, req, "/", http.StatusSeeOther)
		})

	r.Methods("POST").Path("/ip-network-cap").
		HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Size is mostly arbitrary. This is way bigger than we
			// actually need, but it's still tiny and means we don't
			// have to think to see that it's big enough:
			limitedBody := io.LimitReader(req.Body, 512)
			buf, err := ioutil.ReadAll(limitedBody)
			if err != nil {
				println(err.Error())
				w.WriteHeader(400)
				return
			}
			results, err := sessionCtx.ClaimRequest(
				v.Ctx,
				func(params grain.SessionContext_claimRequest_Params) error {
					params.SetRequestToken(string(buf))
					return nil
				}).Struct()
			if err != nil {
				println(err.Error())
				w.WriteHeader(400)
				return
			}
			capability, err := results.Cap()
			if err != nil {
				println(err.Error())
				w.WriteHeader(400)
				return
			}
			v.Backend.IpNetworkCaps <- capability
			return
		})

	r.Methods("GET").Path("/connect").Headers("Upgrade", "websocket").
		Handler(websocket.Handler(func(conn *websocket.Conn) {
			v.Backend.ClientConns <- conn
			for {
				// XXX There's a bug in my websession pacakge that causes the
				// connection to be dropped if this function returns. We're
				// leaking goroutines on every connection now, but this at
				// least "works." Fixing the bug and getting rid of this
				// resource leak is high on my list of priorities.
				time.Sleep(time.Second)
			}
		}))

	session := ws_capnp.WebSession_ServerToClient(websession.FromHandler(v.Ctx, r))
	args.Results.SetSession(grain.UiSession{Client: session.Client})
	return nil
}

func (h UiView) GetViewInfo(p grain.UiView_getViewInfo) error {
	return nil
}

func (h UiView) NewRequestSession(p grain.UiView_newRequestSession) error {
	return nil
}

func (h UiView) NewOfferSession(p grain.UiView_newOfferSession) error {
	return nil
}

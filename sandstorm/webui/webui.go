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
	"os"
	grain_capnp "zenhack.net/go/sandstorm/capnp/grain"
	"zenhack.net/go/sandstorm/grain"
	"zombiezen.com/go/capnproto2"
)

var (
	staticPath = os.Getenv("II_STATIC_ASSET_PATH")
	indexTpl   = template.Must(template.ParseFiles(staticPath + "templates/index.html"))

	errBadXSRFToken      = errors.New("Bad XSRF Token")
	errIllegalPortNumber = errors.New("Illegal Port Number (must be non-zero)")
)

// A ServerConfig specifies a server to connect to.
type ServerConfig struct {
	Host string // Hostname of the server
	Port uint16 // TCP port number
	TLS  bool   // Whether to connect via TLS
}

func (s *ServerConfig) String() string {
	return net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
}

// A Backend is an interface for communication between the UI and the backend.
type Backend struct {
	IpNetworkCaps                    chan capnp.Pointer
	GetServerConfig, SetServerConfig chan ServerConfig
	ClientConns                      chan io.ReadWriteCloser
	HaveNetwork                      chan bool
}

// A SettingsForm a set of values for the "settings" form on the web ui.
type SettingsForm struct {
	Config    ServerConfig
	XSRFToken string
}

type templateContext struct {
	Form        *SettingsForm
	HaveNetwork bool
}

// Validate the SettingsForm. This both sanity-checks the ServerConfig and
// verifies the XSRF token.
func (form *SettingsForm) Validate(xsrfKey string) error {
	if !xsrftoken.Valid(form.XSRFToken, xsrfKey, "TODO", "/proxy-settings") {
		return errBadXSRFToken
	}
	if form.Config.Port == 0 {
		return errIllegalPortNumber
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

// NewHandler returns a new http.Handler serving the web ui and communicating
// with 'backend'.
func NewHandler(ctx context.Context, backend *Backend) (http.Handler, error) {
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
			indexTpl.Execute(w, &templateContext{
				HaveNetwork: <-backend.HaveNetwork,
				Form: &SettingsForm{
					Config:    <-backend.GetServerConfig,
					XSRFToken: token,
				},
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
			backend.SetServerConfig <- form.Config
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
			sessionCtx := w.(grain.HasSessionContext).GetSessionContext()
			results, err := sessionCtx.ClaimRequest(
				ctx,
				func(params grain_capnp.SessionContext_claimRequest_Params) error {
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
			backend.IpNetworkCaps <- capability
			return
		})

	r.Methods("GET").Path("/connect").Headers("Upgrade", "websocket").
		Handler(websocket.Handler(func(conn *websocket.Conn) {
			// The websocket package closes conn when this function returns,
			// so we can't return until the client connection is closed.
			rwcc := newContextRWC(ctx, conn)
			backend.ClientConns <- rwcc
			<-rwcc.Done()
		}))

	return r, nil
}

// A wrapper around io.ReadWriteCloser and a cancellable context, which
// cancels context when closed.
type contextRWC struct {
	context.Context
	cancelFn context.CancelFunc
	io.ReadWriteCloser
}

func newContextRWC(ctx context.Context, rwc io.ReadWriteCloser) contextRWC {
	cancelCtx, cancelFn := context.WithCancel(ctx)
	return contextRWC{cancelCtx, cancelFn, rwc}
}

func (c contextRWC) Close() error {
	err := c.ReadWriteCloser.Close()
	c.cancelFn()
	return err
}

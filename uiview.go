package main

import (
	"zenhack.net/go/sandstorm/capnp/sandstorm/grain"
)

type UiView struct{}

func (v UiView) GetViewInfo(p grain.UiView_getViewInfo) error {
	return nil
}

func (v UiView) NewSession(p grain.UiView_newSession) error {
	// TODO: yield a WebSession
	return nil
}

func (v UiView) NewRequestSession(p grain.UiView_newRequestSession) error {
	return nil
}

func (v UiView) NewOfferSession(p grain.UiView_newOfferSession) error {
	return nil
}

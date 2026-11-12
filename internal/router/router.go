package router

import "net/http"

type RouteRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
}

func New(registrars ...RouteRegistrar) http.Handler {
	mux := http.NewServeMux()
	for _, r := range registrars {
		r.RegisterRoutes(mux)
	}
	return mux
}

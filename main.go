package main

import (
	"context"
	"net/http"

	"github.com/navikt/nada-bucket-proxy/pkg/api"
)

func main() {
	ctx := context.Background()

	router, err := api.NewRouter(ctx)
	if err != nil {
		panic(err)
	}

	server := http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}

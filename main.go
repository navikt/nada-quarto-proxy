package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/go-chi/chi"
	"google.golang.org/api/iterator"
)

type Config struct {
	BucketName string
}

type API struct {
	gcsClient  *storage.Client
	bucketName string
	router     *chi.Mux
}

func NewAPI(ctx context.Context, bucketName string) (*API, error) {
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	router := chi.NewRouter()
	api := &API{
		gcsClient:  gcsClient,
		bucketName: bucketName,
		router:     router,
	}
	api.setupRoutes(router)

	return api, nil
}

func (a *API) setupRoutes(router *chi.Mux) {
	router.Route("/quarto", func(r chi.Router) {
		r.Get("/{id}", a.GetQuartoRedirect)
		r.Route("/{id}/", func(r chi.Router) {
			r.Get("/*", a.GetQuarto)
		})
	})
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BucketName, "bucket", os.Getenv("GCS_QUARTO_BUCKET"), "The quarto storage bucket")
	ctx := context.Background()

	api, err := NewAPI(ctx, cfg.BucketName)
	if err != nil {
		panic(err)
	}

	server := http.Server{
		Addr:    ":8080",
		Handler: api.router,
	}

	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}

func (a *API) GetQuartoRedirect(w http.ResponseWriter, r *http.Request) {
	qID := strings.TrimLeft(r.URL.Path, "/quarto")

	objs := a.gcsClient.Bucket(a.bucketName).Objects(r.Context(), &storage.Query{Prefix: qID + "/"})
	objPath, err := findIndexPage(qID, objs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	http.Redirect(w, r, objPath, http.StatusSeeOther)
}

func (a *API) GetQuarto(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimLeft(r.URL.Path, "/quarto")

	obj := a.gcsClient.Bucket(a.bucketName).Object(path)
	reader, err := obj.NewReader(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	datab, err := ioutil.ReadAll(reader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	switch {
	case strings.HasSuffix(path, ".html"):
		w.Header().Add("content-type", "text/html")
	case strings.HasSuffix(path, ".css"):
		w.Header().Add("content-type", "text/css")
	case strings.HasSuffix(path, ".js"):
		w.Header().Add("content-type", "application/javascript")
	case strings.HasSuffix(path, ".json"):
		w.Header().Add("content-type", "application/json")
	}

	w.Write(datab)
}

func findIndexPage(qID string, objs *storage.ObjectIterator) (string, error) {
	page := ""
	for {
		o, err := objs.Next()
		if err == iterator.Done {
			if page == "" {
				return "", fmt.Errorf("could not find html for id %v", qID)
			}
			return page, nil
		}

		if strings.HasSuffix(o.Name, "/index.html") {
			return o.Name, nil
		} else if strings.HasSuffix(o.Name, ".html") {
			page = o.Name
		}
	}
}

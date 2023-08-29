package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/go-chi/chi"
	"google.golang.org/api/iterator"
)

type API struct {
	gcsClient  *storage.Client
	bucketName string
	quartoUUID string
	quartoPath string
}

func NewRouter(ctx context.Context) (*chi.Mux, error) {
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	bucketName := os.Getenv("GCS_QUARTO_BUCKET")
	if bucketName == "" {
		return nil, fmt.Errorf("no bucket name configured")
	}

	quartoUUID := os.Getenv("QUARTO_UUID")
	if quartoUUID == "" {
		return nil, fmt.Errorf("no quarto UUID configured")
	}

	quartoPath := os.Getenv("QUARTO_PATH")
	if quartoPath == "" {
		return nil, fmt.Errorf("no quarto path configured")
	}

	router := chi.NewRouter()
	api := &API{
		gcsClient:  gcsClient,
		bucketName: bucketName,
		quartoUUID: quartoUUID,
		quartoPath: quartoPath,
	}
	api.setupRoutes(router)

	return router, nil
}

func (a *API) setupRoutes(router *chi.Mux) {
	if a.quartoPath == "omverdensanalyse" {
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/"+a.quartoPath, http.StatusSeeOther)
		})
	}

	router.Route("/"+a.quartoPath, func(r chi.Router) {
		r.Use(a.quartoMiddleware)
		r.Get("/*", a.GetQuarto)
	})
}

func (a *API) quartoMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		regex, _ := regexp.Compile(`[\n]*\.[\n]*`) // check if object path has file extension
		if !regex.MatchString(r.URL.Path) {
			a.redirect(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *API) redirect(w http.ResponseWriter, r *http.Request) {
	objects := a.gcsClient.Bucket(a.bucketName).Objects(r.Context(), &storage.Query{Prefix: a.quartoUUID + "/"})
	objPath, err := a.findIndexPage(objects)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	r.URL.Path = "/"
	http.Redirect(w, r, objPath, http.StatusSeeOther)
}

func (a *API) GetQuarto(w http.ResponseWriter, r *http.Request) {
	path := a.quartoUUID + "/" + strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/%v/", a.quartoPath))
	attr, objBytes, err := a.GetObject(r.Context(), path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("content-type", attr.ContentType)
	w.Header().Add("content-length", strconv.Itoa(int(attr.Size)))
	w.Header().Add("content-encoding", attr.ContentEncoding)

	w.Write(objBytes)
}

func (a *API) GetObject(ctx context.Context, path string) (*storage.ObjectAttrs, []byte, error) {
	obj := a.gcsClient.Bucket(a.bucketName).Object(path)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, []byte{}, err
	}

	datab, err := io.ReadAll(reader)
	if err != nil {
		return nil, []byte{}, err
	}

	attr, err := obj.Attrs(ctx)
	if err != nil {
		return nil, []byte{}, err
	}

	return attr, datab, nil
}

func (a *API) findIndexPage(objects *storage.ObjectIterator) (string, error) {
	page := ""
	for {
		object, err := objects.Next()
		if err == iterator.Done {
			if page == "" {
				return "", fmt.Errorf("could not find html for quarto")
			}

			return page, nil
		}

		if err != nil {
			return "", fmt.Errorf("index page not found")
		}

		if strings.HasSuffix(strings.ToLower(object.Name), "/index.html") {
			return a.quartoPath + "/" + strings.TrimPrefix(object.Name, a.quartoUUID+"/"), nil
		} else if strings.HasSuffix(strings.ToLower(object.Name), ".html") {
			page = a.quartoPath + "/" + strings.TrimPrefix(object.Name, a.quartoUUID+"/")
		}
	}
}

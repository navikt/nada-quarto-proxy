package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/go-chi/chi"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

type Config struct {
	BucketName string
}

type API struct {
	gcsClient  *storage.Client
	bucketName string
	router     *chi.Mux
	log        *logrus.Entry
	quartoUUID string
}

func NewAPI(ctx context.Context, bucketName string, log *logrus.Entry) (*API, error) {
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	quartoUUID := os.Getenv("QUARTO_UUID")
	if quartoUUID == "" {
		return nil, fmt.Errorf("no quarto UUID configured %v", quartoUUID)
	}

	router := chi.NewRouter()
	api := &API{
		gcsClient:  gcsClient,
		bucketName: bucketName,
		router:     router,
		log:        log,
		quartoUUID: quartoUUID,
	}
	api.setupRoutes(router)

	return api, nil
}

func (a *API) setupRoutes(router *chi.Mux) {
	router.Route("/omverdensanalyse", func(r chi.Router) {
		r.Use(a.QuartoMiddleware)
		r.Get("/*", a.GetQuarto)
	})
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BucketName, "bucket", os.Getenv("GCS_QUARTO_BUCKET"), "The quarto storage bucket")
	ctx := context.Background()
	log := logrus.New()

	api, err := NewAPI(ctx, cfg.BucketName, log.WithField("subsystem", "api"))
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

func (a *API) QuartoMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		regex, _ := regexp.Compile(`[\n]*\.[\n]*`) // check if object path has file extension
		if !regex.MatchString(r.URL.Path) {
			a.Redirect(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *API) Redirect(w http.ResponseWriter, r *http.Request) {
	objs := a.gcsClient.Bucket(a.bucketName).Objects(r.Context(), &storage.Query{Prefix: a.quartoUUID + "/"})
	objPath, err := a.findIndexPage(a.quartoUUID, objs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	fmt.Println("index page", objPath)

	path := strings.TrimPrefix(objPath, a.quartoUUID+"/")

	http.Redirect(w, r, path, http.StatusSeeOther)
}

func (a *API) GetQuarto(w http.ResponseWriter, r *http.Request) {
	fmt.Println("get quarto", r.URL.Path)

	path := a.convertPath(r.URL.Path)
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

	datab, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, []byte{}, err
	}

	attr, err := obj.Attrs(ctx)
	if err != nil {
		return nil, []byte{}, err
	}

	return attr, datab, nil
}

func (a *API) findIndexPage(qID string, objs *storage.ObjectIterator) (string, error) {
	page := ""
	for {
		o, err := objs.Next()
		if err == iterator.Done {
			if page == "" {
				return "", fmt.Errorf("could not find html for id %v", qID)
			}
			return "omverdensanalyse/" + page, nil
		}
		if err != nil {
			a.log.WithError(err).Error("searching for index page in bucket")
			return "", fmt.Errorf("index page not found")
		}

		if strings.HasSuffix(strings.ToLower(o.Name), "/index.html") {
			return o.Name, nil
		} else if strings.HasSuffix(strings.ToLower(o.Name), ".html") {
			page = "omverdensanalyse/" + o.Name
		}
	}
}

func (a *API) convertPath(urlPath string) string {
	parts := strings.Split(urlPath, "/")
	out := a.quartoUUID
	for _, p := range parts {
		if p == "omverdensanalyse" || p == "" {
			continue
		}
		out = out + "/" + p
	}
	fmt.Println("path converted", out)
	return out
}

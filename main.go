package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
)

type Config struct {
	BucketName string
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BucketName, "bucket", os.Getenv("GCS_QUARTO_BUCKET"), "The quarto storage bucket")

	ctx := context.Background()

	router := gin.New()

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		panic(err)
	}

	setupRoutes(router, gcsClient, cfg.BucketName)

	if err := router.Run(); err != nil {
		panic(err)
	}
}

func setupRoutes(router *gin.Engine, gcsClient *storage.Client, bucketName string) {
	router.GET("/quarto/:id", func(c *gin.Context) {
		qID := c.Param("id")

		objs := gcsClient.Bucket(bucketName).Objects(c, &storage.Query{Prefix: qID + "/"})
		objPath, err := findIndexPage(qID, objs)
		if err != nil {
			c.JSON(http.StatusNotFound, map[string]string{"status": fmt.Sprintf("quarto with id %v not found", qID)})
		}

		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/quarto/%v", objPath))
	})

	router.GET("/quarto/:id/*file", func(c *gin.Context) {
		qID := c.Param("id")
		qFile := c.Param("file")
		objPath := fmt.Sprintf("%v/%v", qID, strings.TrimLeft(qFile, "/"))

		obj := gcsClient.Bucket(bucketName).Object(objPath)
		data, err := obj.NewReader(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, map[string]string{"status": "internal server error"})
			return
		}
		io.Copy(c.Writer, data)
	})

	// todo: add PUT here nada-backend calls from graphql resolver?
	// nais manifest does not allow to add access to a bucket that is already owned by another app
	// therefore we must create a service account with read permission and inject a service account key
	// for this app as nada-backend owns the bucket.
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

		if o.Name == qID+"/index.html" {
			return o.Name, nil
		} else if strings.HasSuffix(o.Name, ".html") {
			page = o.Name
		}
	}
}

package main

import (
	"log"
	"testing"

	"github.com/k8-proxy/k8-go-comm/pkg/minio"
)

func TestMinioRemoveScheduler(t *testing.T) {
	var err error
	minioEndpoint := "localhost:9000"
	minioAccessKey := "minioadmin"
	minioSecretKey := "minioadmin"
	sourceMinioBucket = "test"
	minioClient, err = minio.NewMinioClient(minioEndpoint, minioAccessKey, minioSecretKey, false)

	if err != nil {
		log.Println("could not s start minio client ")
	}
	minioRemoveScheduler(sourceMinioBucket, "")

}

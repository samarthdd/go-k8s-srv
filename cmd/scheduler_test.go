package main

import (
	"log"
	"testing"

	"github.com/k8-proxy/k8-go-comm/pkg/minio"
)

func TestMinioRemoveScheduler(t *testing.T) {
	var err error
	if minioEndpoint != "" {
		minioClient, err = minio.NewMinioClient(minioEndpoint, minioAccessKey, minioSecretKey, false)

		if err != nil {
			log.Println("could not s start minio client ")
		}
		minioRemoveScheduler(sourceMinioBucket, "")
	}
}

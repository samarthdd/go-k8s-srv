package main

import (
	"log"
	"testing"
	"time"

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
func TestTickerConf(t *testing.T) {
	defaultTickerDuration := 30 * time.Minute
	oneYear := 60 * 24 * 365 * time.Minute

	tickerTest := []struct {
		text string
		dur  time.Duration
	}{
		{"", defaultTickerDuration},
		{"nonvalid", defaultTickerDuration},
		{"0", oneYear},
		{"15", 15 * time.Minute},
		{"1.5", defaultTickerDuration},
		{"-11", oneYear},
	}
	for _, v := range tickerTest {
		res := tickerConf(v.text)
		if res != v.dur {

			t.Errorf("fails expected for the string   %s", v.text)

		}
	}

}

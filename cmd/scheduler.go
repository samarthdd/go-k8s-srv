package main

import (
	"context"
	"os"
	"strconv"
	"time"

	zlog "github.com/rs/zerolog/log"

	"github.com/minio/minio-go/v7"
	miniov7 "github.com/minio/minio-go/v7"
)

func minioRemoveScheduler(bucketName, prefix string) {

	//timer := time.NewTimer(10 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	// Send object names that are needed to be removed to objectsCh
	var object <-chan miniov7.ObjectInfo

	// List all objects from a bucket-name with a matching prefix.

	object = minioClient.ListObjects(ctx, bucketName, miniov7.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	opts := minio.RemoveObjectsOptions{
		GovernanceBypass: true,
	}

	for rErr := range minioClient.RemoveObjects(ctx, bucketName, object, opts) {
		zlog.Error().Err(rErr.Err).Msg("Error detected during deletion")
	}

}

func ticker(done <-chan bool) {
	tickerDuration := tickerConf(os.Getenv("MINIO_DELETE_FILE_DURATION"))

	ticker := time.NewTicker(tickerDuration)

	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
			case <-ticker.C:
				zlog.Info().Msg("the origin files and rebuild file are being deleted")
				minioRemoveScheduler(sourceMinioBucket, "")
				minioRemoveScheduler(cleanMinioBucket, "")

			}
		}
	}()

}

func tickerConf(d string) time.Duration {
	defaultTickerDuration := 30 * time.Minute
	oneYear := 60 * 24 * 365 * time.Minute

	i, err := strconv.Atoi(d)
	if err != nil {
		return defaultTickerDuration
	}
	if i > 0 {
		dur := time.Duration(i) * time.Minute
		return dur
	}
	if i < 1 {
		return oneYear
	}
	return defaultTickerDuration
}

package main

import (
	"context"
	"log"
	"strconv"
	"time"

	zlog "github.com/rs/zerolog/log"

	"github.com/minio/minio-go/v7"
	miniov7 "github.com/minio/minio-go/v7"
)

func minioRemoveScheduler(bucketName, prefix string) {

	//timer := time.NewTimer(10 * time.Second)
	now := time.Now()
	Lastmodified := tickerConf(DeleteDuration)
	then := now.Add(time.Duration(-Lastmodified))
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	// Send object names that are needed to be removed to objectsCh
	objectsCh := make(chan miniov7.ObjectInfo)

	// Send object names that are needed to be removed to objectsCh
	go func() {
		defer close(objectsCh)
		// List all objects from a bucket-name with a matching prefix.
		for object := range minioClient.ListObjects(ctx, "test", miniov7.ListObjectsOptions{
			Prefix:    "",
			Recursive: true,
		}) {
			if object.Err != nil {
				log.Fatalln(object.Err)
			}
			// filter LastModified
			if object.LastModified.Before(then) == true {
				objectsCh <- object
			}
		}

	}()

	opts := minio.RemoveObjectsOptions{
		GovernanceBypass: true,
	}

	for rErr := range minioClient.RemoveObjects(ctx, bucketName, objectsCh, opts) {
		zlog.Error().Err(rErr.Err).Msg("Error detected during deletion")
	}

}

func ticker(done <-chan bool) {
	tickerDuration := tickerConf(DeleteDuration)
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

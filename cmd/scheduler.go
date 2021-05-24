package main

import (
	"context"
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
		Recursive: false,
	})

	opts := minio.RemoveObjectsOptions{
		GovernanceBypass: true,
	}

	for rErr := range minioClient.RemoveObjects(ctx, bucketName, object, opts) {
		zlog.Error().Err(rErr.Err).Msg("Error detected during deletion")
	}

}

func ticker(done <-chan bool) {

	ticker := time.NewTicker(10 * time.Minute)

	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
			case <-ticker.C:
				zlog.Info().Msg("the origin files and rebuild file are being deleted")
				minioRemoveScheduler(sourceMinioBucket, "")
				minioRemoveScheduler(cleanMinioBucket, "rebuild-")

			}
		}
	}()

}

func syncher() {
	//block upload to minio source bucket until all the files deleted
	//sleep for 2 seconds for a secuity reason
}

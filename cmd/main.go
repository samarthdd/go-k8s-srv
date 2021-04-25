package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/k8-proxy/k8-go-comm/pkg/minio"
	"github.com/k8-proxy/k8-go-comm/pkg/rabbitmq"
	zlog "github.com/rs/zerolog/log"
	"github.com/streadway/amqp"

	miniov7 "github.com/minio/minio-go/v7"
)

var (
	AdpatationReuquestExchange   = "adaptation-exchange"
	AdpatationReuquestRoutingKey = "adaptation-request"
	AdpatationReuquestQueueName  = "adaptation-request-queue"

	ProcessingRequestExchange   = "processing-request-exchange"
	ProcessingRequestRoutingKey = "processing-request"
	ProcessingRequestQueueName  = "processing-request"

	inputMount                     = os.Getenv("INPUT_MOUNT")
	adaptationRequestQueueHostname = os.Getenv("ADAPTATION_REQUEST_QUEUE_HOSTNAME")
	adaptationRequestQueuePort     = os.Getenv("ADAPTATION_REQUEST_QUEUE_PORT")
	messagebrokeruser              = os.Getenv("MESSAGE_BROKER_USER")
	messagebrokerpassword          = os.Getenv("MESSAGE_BROKER_PASSWORD")

	minioEndpoint     = os.Getenv("MINIO_ENDPOINT")
	minioAccessKey    = os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey    = os.Getenv("MINIO_SECRET_KEY")
	sourceMinioBucket = os.Getenv("MINIO_SOURCE_BUCKET")
	cleanMinioBucket  = os.Getenv("MINIO_CLEAN_BUCKET")

	ProcessingOutcomeExchange   = "processing-outcome-exchange"
	ProcessingOutcomeRoutingKey = "processing-outcome"
	ProcessingOutcomeQueueName  = "processing-outcome-queue"

	AdaptationOutcomeExchange   = "adaptation-exchange"
	AdaptationOutcomeRoutingKey = "adaptation-exchange"
	AdaptationOutcomeQueueName  = "amq.rabbitmq.reply-to"

	minioClient *miniov7.Client
	connection  *amqp.Connection
)

func main() {

	// Get a connection
	var err error

	connection, err = rabbitmq.NewInstance(adaptationRequestQueueHostname, adaptationRequestQueuePort, messagebrokeruser, messagebrokerpassword)
	if err != nil {
		zlog.Fatal().Err(err).Msg("could not start rabbitmq connection ")
	}

	// Initiate a publisher on processing exchange

	// Start a consumer
	msgs, ch, err := rabbitmq.NewQueueConsumer(connection, AdpatationReuquestQueueName, AdpatationReuquestExchange, AdpatationReuquestRoutingKey)
	if err != nil {
		zlog.Fatal().Err(err).Msg("could not start  AdpatationReuquest consumer ")
	}
	defer ch.Close()

	outMsgs, outChannel, err := rabbitmq.NewQueueConsumer(connection, ProcessingOutcomeQueueName, ProcessingOutcomeExchange, ProcessingOutcomeRoutingKey)
	if err != nil {
		zlog.Fatal().Err(err).Msg("could not start ProcessingOutcome consumer ")

	}
	defer outChannel.Close()

	minioClient, err = minio.NewMinioClient(minioEndpoint, minioAccessKey, minioSecretKey, false)

	if err != nil {
		zlog.Fatal().Err(err).Msg("could not start minio client ")
	}

	err = createBucketIfNotExist(sourceMinioBucket)
	if err != nil {
		zlog.Error().Err(err).Msg(" sourceMinioBucket createBucketIfNotExist error")
	}

	err = createBucketIfNotExist(cleanMinioBucket)
	if err != nil {
		zlog.Error().Err(err).Msg("cleanMinioBucket createBucketIfNotExist error")
	}

	forever := make(chan bool)

	// Consume
	go func() {
		for d := range msgs {
			zlog.Info().Msg("adaptation request consumer  received message from queue ")

			err := processMessage(d)
			if err != nil {
				zlog.Error().Err(err).Msg("error adaptationRequest consumer Failed to process message")
			}
		}
	}()

	go func() {
		for d := range outMsgs {
			zlog.Info().Msg(" processingOutcome consumer received message from queue ")

			err := outcomeProcessMessage(d)
			if err != nil {
				zlog.Error().Err(err).Msg("error processingOutcome consumer Failed to process message")
			}
		}
	}()

	log.Printf("Waiting for messages")
	<-forever

}

func createBucketIfNotExist(bucketName string) error {
	exist, err := minio.CheckIfBucketExists(minioClient, bucketName)
	if err != nil {

		return fmt.Errorf("error creating source  minio bucket : %s", err)
	}
	if !exist {

		err := minio.CreateNewBucket(minioClient, bucketName)
		if err != nil {
			return fmt.Errorf("error could not create minio bucket : %s", err)
		}
	}
	return nil
}

func processMessage(d amqp.Delivery) error {

	if d.Headers["file-id"] == nil ||
		d.Headers["source-file-location"] == nil ||
		d.Headers["rebuilt-file-location"] == nil {
		return fmt.Errorf("Headers value is nil")
	}

	publisher, err := rabbitmq.NewQueuePublisher(connection, ProcessingRequestExchange)
	if err != nil {
		return fmt.Errorf("error  starting  Processing Request publisher : %s", err)
	}
	defer publisher.Close()

	fileID := d.Headers["file-id"].(string)
	input := d.Headers["source-file-location"].(string)
	sourcef := d.Headers["rebuilt-file-location"].(string)

	zlog.Info().Str("fileID", fileID).Str("rebuilt-file-location", input).Str("source-file-location", sourcef).Msg("")

	// Upload the source file to Minio and Get presigned URL
	sourcePresignedURL, err := minio.UploadAndReturnURL(minioClient, sourceMinioBucket, input, time.Second*60*60*24)
	if err != nil {
		return fmt.Errorf("error uploading file from minio : %s", err)
	}
	zlog.Info().Msg("file uploaded to minio successfully")

	d.Headers["source-presigned-url"] = sourcePresignedURL.String()

	d.Headers["reply-to"] = d.ReplyTo

	// Publish the details to Rabbit
	err = rabbitmq.PublishMessage(publisher, ProcessingRequestExchange, ProcessingRequestRoutingKey, d.Headers, []byte(""))
	if err != nil {
		return fmt.Errorf("error publish to Processing request queue : %s", err)
	}

	zlog.Info().Str("Exchange", ProcessingRequestExchange).Str("RoutingKey", ProcessingRequestRoutingKey).Msg("message published to queue ")

	return nil
}

func outcomeProcessMessage(d amqp.Delivery) error {

	if d.Headers["clean-presigned-url"] == nil ||
		d.Headers["rebuilt-file-location"] == nil ||
		d.Headers["reply-to"] == nil {
		return fmt.Errorf("Headers value is nil")
	}

	publisher, err := rabbitmq.NewQueuePublisher(connection, ProcessingRequestExchange)
	if err != nil {
		return fmt.Errorf("error  starting  adaptation outcome publisher : %s", err)
	}
	defer publisher.Close()

	cleanPresignedURL := d.Headers["clean-presigned-url"].(string)
	outputFileLocation := d.Headers["rebuilt-file-location"].(string)

	// Download the file to output file location
	err = minio.DownloadObject(cleanPresignedURL, outputFileLocation)
	if err != nil {

		return fmt.Errorf("error downloading file from minio : %s", err)
	}
	zlog.Info().Msg("file downloaded from minio successfully")

	d.Headers["file-outcome"] = "replace"
	// Publish the details to Rabbit

	//because of missmatch configuration in icap-service  , the exchange and routing keys are null
	AdaptationOutcomeExchange = ""
	AdaptationOutcomeRoutingKey = d.Headers["reply-to"].(string)

	err = rabbitmq.PublishMessage(publisher, AdaptationOutcomeExchange, AdaptationOutcomeRoutingKey, d.Headers, []byte(""))
	if err != nil {
		return fmt.Errorf("error publish to adaption outcome queue : %s", err)
	}
	zlog.Info().Str("Exchange", AdaptationOutcomeExchange).Str("RoutingKey", AdaptationOutcomeRoutingKey).Msg("message published to queue ")

	return nil
}

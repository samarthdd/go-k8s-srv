package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/k8-proxy/k8-go-comm/pkg/minio"
	"github.com/k8-proxy/k8-go-comm/pkg/rabbitmq"
	zlog "github.com/rs/zerolog/log"
	"github.com/streadway/amqp"

	miniov7 "github.com/minio/minio-go/v7"

	"github.com/k8-proxy/go-k8s-srv/tracing"
	"github.com/opentracing/opentracing-go"
)

var (
	AdpatationReuquestExchange   = "adaptation-exchange"
	AdpatationReuquestRoutingKey = "adaptation-request"
	AdpatationReuquestQueueName  = "adaptation-request-queue"

	ProcessingRequestExchange   = "processing-request-exchange"
	ProcessingRequestRoutingKey = "processing-request"
	ProcessingRequestQueueName  = "processing-request"

	ProcessingOutcomeExchange   = "processing-outcome-exchange"
	ProcessingOutcomeRoutingKey = "processing-outcome"
	ProcessingOutcomeQueueName  = "processing-outcome-queue"

	AdaptationOutcomeExchange   = "adaptation-exchange"
	AdaptationOutcomeRoutingKey = "adaptation-exchange"
	AdaptationOutcomeQueueName  = "amq.rabbitmq.reply-to"

	inputMount                     = os.Getenv("INPUT_MOUNT")
	adaptationRequestQueueHostname = os.Getenv("ADAPTATION_REQUEST_QUEUE_HOSTNAME")
	adaptationRequestQueuePort     = os.Getenv("ADAPTATION_REQUEST_QUEUE_PORT")
	messagebrokeruser              = os.Getenv("MESSAGE_BROKER_USER")
	messagebrokerpassword          = os.Getenv("MESSAGE_BROKER_PASSWORD")

	TikaRequestExange           = "comparison-request-exchange"
	ComparisonRequestRoutingKey = "comparison-request"

	minioEndpoint        = os.Getenv("MINIO_ENDPOINT")
	minioAccessKey       = os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey       = os.Getenv("MINIO_SECRET_KEY")
	sourceMinioBucket    = os.Getenv("MINIO_SOURCE_BUCKET")
	cleanMinioBucket     = os.Getenv("MINIO_CLEAN_BUCKET")
	transactionStorePath = os.Getenv("TRANSACTION_STORE_PATH")
	minioClient          *miniov7.Client
	connection           *amqp.Connection
	JeagerStatus         bool
	tikasataus           bool
)

const thisServiceName = "GWFileProcess"

type amqpHeadersCarrier map[string]interface{}

var ctx context.Context
var ProcessTracer opentracing.Tracer
var ProcessTracer2 opentracing.Tracer
var helloTo string

const (
	presignedUrlExpireIn = time.Hour * 24
)

func main() {
	tikasatausenv := os.Getenv("TIKA_COMPARISON_ON")
	if tikasatausenv == "true" {
		tikasataus = true
	} else {
		tikasataus = false
	}

	JeagerStatusEnv := os.Getenv("JAEGER_AGENT_ON")
	if JeagerStatusEnv == "true" {
		JeagerStatus = true
	} else {
		JeagerStatus = false
	}

	if transactionStorePath == "" {
		zlog.Fatal().Msg("TRANSACTION_STORE_PATH is not configured ")
	}
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

	done := make(chan bool)
	ticker(done)

	forever := make(chan bool)

	// Consume
	go func() {
		for d := range msgs {

			zlog.Info().Msg("adaptation request consumer  received message from queue ")

			err := processMessage(d)
			if err != nil {
				processend(err)
				zlog.Error().Err(err).Msg("error adaptationRequest consumer Failed to process message")
			}
		}
	}()

	go func() {
		for d := range outMsgs {

			zlog.Info().Msg(" processingOutcome consumer received message from queue ")

			err := outcomeProcessMessage(d)

			if err != nil {
				processend(err)
				zlog.Error().Err(err).Msg("error processingOutcome consumer Failed to process message")
			}
		}
	}()

	log.Printf("Waiting for messages")
	<-forever
	<-done
}
func processend(err error) {
	if JeagerStatus == true && ctx != nil {
		fmt.Println(err)

		span, _ := opentracing.StartSpanFromContext(ctx, "ProcessingEndError")
		defer span.Finish()
		span.LogKV("event", err)
	}

}

func processMessage(d amqp.Delivery) error {
	if JeagerStatus == true {
		tracer, closer := tracing.Init(thisServiceName)
		defer closer.Close()
		opentracing.SetGlobalTracer(tracer)
		ProcessTracer = tracer
	}

	if d.Headers["file-id"] == nil ||
		d.Headers["source-file-location"] == nil ||
		d.Headers["rebuilt-file-location"] == nil {
		return fmt.Errorf("Headers value is nil")
	}
	var span opentracing.Span

	if JeagerStatus == true {
		helloTo = d.Headers["file-id"].(string)
		span = ProcessTracer.StartSpan("ProcessFile")
		span.SetTag("file-id", helloTo)
		defer span.Finish()

		ctx = opentracing.ContextWithSpan(context.Background(), span)
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

	sourcePresignedURL, err := minio.UploadAndReturnURL(minioClient, sourceMinioBucket, input, presignedUrlExpireIn)
	if err != nil {
		return fmt.Errorf("error uploading file from minio : %s", err)
	}
	zlog.Info().Msg("file uploaded to minio successfully")
	d.Headers["source-presigned-url"] = sourcePresignedURL.String()
	d.Headers["reply-to"] = d.ReplyTo
	headers := d.Headers

	if JeagerStatus == true {
		if err := Inject(span, headers); err != nil {
			return err
		}
	}
	// Publish the details to Rabbit
	err = rabbitmq.PublishMessage(publisher, ProcessingRequestExchange, ProcessingRequestRoutingKey, headers, []byte(""))
	if err != nil {
		return fmt.Errorf("error publish to Processing request queue : %s", err)
	}

	zlog.Info().Str("Exchange", ProcessingRequestExchange).Str("RoutingKey", ProcessingRequestRoutingKey).Msg("message published to queue ")

	return nil
}

func outcomeProcessMessage(d amqp.Delivery) error {
	if JeagerStatus == true {
		tracer, closer := tracing.Init("outMsgs")
		defer closer.Close()
		opentracing.SetGlobalTracer(tracer)
		ProcessTracer2 = tracer
	}
	if JeagerStatus == true {

		if d.Headers["uber-trace-id"] != nil {
			fmt.Println("uber-trace-id")
			spCtx, ctxsuberr := ExtractWithTracer(d.Headers, ProcessTracer2)
			if spCtx == nil {
				fmt.Println("cpctxsub nil 1")
			}
			if ctxsuberr != nil {
				fmt.Println(ctxsuberr)
			}
			// Extract the span context out of the AMQP header.
			sp := opentracing.StartSpan(
				"outcomeProcessMessage",
				opentracing.FollowsFrom(spCtx),
			)
			if d.Headers["file-id"] == nil {
				helloTo = "nil-file-id"
			} else {
				helloTo = d.Headers["file-id"].(string)
			}
			sp.SetTag("file-clean", helloTo)
			defer sp.Finish()
			ctxsubtx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
			defer cancel()
			// Update the context with the span for the subsequent reference.
			ctx = opentracing.ContextWithSpan(ctxsubtx, sp)
		} else {
			fmt.Println("no-uber-trace-id")
			if d.Headers["file-id"] == nil {
				helloTo = "nil-file-id"
			} else {
				helloTo = d.Headers["file-id"].(string)
			}
			span := ProcessTracer2.StartSpan("outcomeProcessMessage")
			span.SetTag("file-id", helloTo)
			defer span.Finish()

			ctx = opentracing.ContextWithSpan(context.Background(), span)

		}
	}

	fileID := d.Headers["file-id"].(string)
	cleanPresignedURL, _ := d.Headers["clean-presigned-url"].(string)
	outputFileLocation, _ := d.Headers["rebuilt-file-location"].(string)
	reportFileName := "report.xml"
	metadataFileName := "metadata.json"

	publisher, err := rabbitmq.NewQueuePublisher(connection, ProcessingRequestExchange)
	if err != nil {
		return fmt.Errorf("error  starting  adaptation outcome publisher : %s", err)
	}
	defer publisher.Close()
	// Download the file to output file location
	if cleanPresignedURL != "" {
		err = minio.DownloadObject(cleanPresignedURL, outputFileLocation)
		if err != nil {
			return fmt.Errorf("error downloading  rebuilt file from minio : %s", err)
		}
		zlog.Info().Msg("rebuilt file downloaded from minio successfully")
	} else {
		zlog.Info().Msg("there is no rebuilt file to download from minio")
	}
	if tikasataus {
		err = rabbitmq.PublishMessage(publisher, TikaRequestExange, ComparisonRequestRoutingKey, d.Headers, []byte(""))
		if err != nil {
			return fmt.Errorf("error publish to comparison request queue : %s", err)
		}
	}

	if d.Headers["report-presigned-url"] != nil {

		reportPresignedURL, _ := d.Headers["report-presigned-url"].(string)
		reportPath := fmt.Sprintf("%s/%s", transactionStorePath, fileID)

		if _, err := os.Stat(reportPath); os.IsNotExist(err) {
			os.MkdirAll(reportPath, 0777)
		}

		reportFileLocation := fmt.Sprintf("%s/%s", reportPath, reportFileName)

		zlog.Info().Str("report file location ", reportFileLocation).Msg("")

		err := minio.DownloadObject(reportPresignedURL, reportFileLocation)
		if err != nil {
			return err
		}

	} else {
		zlog.Info().Msg("there is no report file to download from minio")
	}

	if d.Headers["metadata-presigned-url"] != nil {
		metadataPresignedURL, _ := d.Headers["metadata-presigned-url"].(string)
		metadataPath := fmt.Sprintf("%s/%s", transactionStorePath, fileID)

		if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
			os.MkdirAll(metadataPath, 0777)
		}

		metadataFileLocation := fmt.Sprintf("%s/%s", metadataPath, metadataFileName)

		zlog.Info().Str("metadata file location ", metadataFileLocation).Msg("")

		err := minio.DownloadObject(metadataPresignedURL, metadataFileLocation)
		if err != nil {
			return err
		}

	} else {
		zlog.Info().Msg("there is no metadata file to download from minio")
	}

	fileOutcome, _ := d.Headers["file-outcome"].(string)
	log.Printf("\033[35m rebuild status is  : %s\n", fileOutcome)

	AdaptationOutcomeExchange = ""
	AdaptationOutcomeRoutingKey, _ = d.Headers["reply-to"].(string)
	if JeagerStatus == true && ctx != nil {
		span, _ := opentracing.StartSpanFromContext(ctx, "AdaptationOutcomeExchange")
		defer span.Finish()
		span.LogKV("event", "outcome")
	}
	err = rabbitmq.PublishMessage(publisher, AdaptationOutcomeExchange, AdaptationOutcomeRoutingKey, d.Headers, []byte(""))
	if err != nil {
		return fmt.Errorf("error publish to adaption outcome queue : %s", err)
	}
	zlog.Info().Str("Exchange", AdaptationOutcomeExchange).Str("RoutingKey", AdaptationOutcomeRoutingKey).Msg("message published to queue ")

	return nil
}

func createBucketIfNotExist(bucketName string) error {
	if JeagerStatus == true && ctx != nil {
		span, _ := opentracing.StartSpanFromContext(ctx, "createBucketIfNotExist")
		defer span.Finish()
		span.LogKV("event", "createBucket")
	}
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

func RemoveProcessedFilesMinio(fileName, BucketName string) {
	if JeagerStatus == true && ctx != nil {
		span, _ := opentracing.StartSpanFromContext(ctx, "RemoveProcessedFilesMinio")
		defer span.Finish()
		span.LogKV("event", "removefile")
	}
	minio.DeleteObjectInMinio(minioClient, BucketName, fileName)

}
func Inject(span opentracing.Span, hdrs amqp.Table) error {
	c := amqpHeadersCarrier(hdrs)
	return span.Tracer().Inject(span.Context(), opentracing.TextMap, c)
}
func Extract(hdrs amqp.Table) (opentracing.SpanContext, error) {
	c := amqpHeadersCarrier(hdrs)
	return opentracing.GlobalTracer().Extract(opentracing.TextMap, c)
}
func (c amqpHeadersCarrier) ForeachKey(handler func(key, val string) error) error {
	for k, val := range c {
		v, ok := val.(string)
		if !ok {
			continue
		}
		if err := handler(k, v); err != nil {
			return err
		}
	}
	return nil
}

// Set implements Set() of opentracing.TextMapWriter.
func (c amqpHeadersCarrier) Set(key, val string) {
	c[key] = val
}
func ExtractWithTracer(hdrs amqp.Table, tracer opentracing.Tracer) (opentracing.SpanContext, error) {
	if tracer == nil {
		return nil, errors.New("tracer is nil")
	}
	c := amqpHeadersCarrier(hdrs)
	return tracer.Extract(opentracing.TextMap, c)
}

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/k8-proxy/go-k8s-srv/tracing"
	"github.com/k8-proxy/k8-go-comm/pkg/rabbitmq"
	min7 "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/opentracing/opentracing-go"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/streadway/amqp"
)

// var body = "body test"
var TestMQTable amqp.Table
var endpoint string
var ResourceMQ *dockertest.Resource
var ResourceMinio *dockertest.Resource
var ResourceJG *dockertest.Resource

var poolMq *dockertest.Pool
var poolMinio *dockertest.Pool
var poolJG *dockertest.Pool

var secretsstring string

func jaegerserver() {
	var errpool error

	poolJG, errpool = dockertest.NewPool("")
	if errpool != nil {
		log.Fatalf("Could not connect to docker: %s", errpool)
	}
	opts := dockertest.RunOptions{
		Repository: "jaegertracing/all-in-one",
		Tag:        "latest",

		PortBindings: map[docker.Port][]docker.PortBinding{
			"5775/udp": {{HostPort: "5775"}},
			"6831/udp": {{HostPort: "6831"}},
			"6832/udp": {{HostPort: "6832"}},
			"5778/tcp": {{HostPort: "5778"}},
			"16686":    {{HostPort: "16686"}},
			"14268":    {{HostPort: "14268"}},
			"9411":     {{HostPort: "9411"}},
		},
	}
	resource, err := poolJG.RunWithOptions(&opts)
	if err != nil {
		log.Fatalf("Could not start resource: %s", err.Error())
	}
	ResourceJG = resource

}
func rabbitserver() {
	var errpool error

	poolMq, errpool = dockertest.NewPool("")
	if errpool != nil {
		log.Fatalf("Could not connect to docker: %s", errpool)
	}
	opts := dockertest.RunOptions{
		Repository: "rabbitmq",
		Tag:        "latest",
		Env: []string{
			"host=root",
		},
		ExposedPorts: []string{"5672"},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"5672": {
				{HostPort: "5672"},
			},
		},
	}
	resource, err := poolMq.RunWithOptions(&opts)
	if err != nil {
		log.Fatalf("Could not start resource: %s", err.Error())
	}
	ResourceMQ = resource

}

// Minio server
func minioserver() {
	var errpool error

	minioAccessKey = secretsstring
	minioSecretKey = secretsstring

	poolMinio, errpool = dockertest.NewPool("")
	if errpool != nil {
		log.Fatalf("Could not connect to docker: %s", errpool)
	}

	options := &dockertest.RunOptions{
		Repository: "minio/minio",
		Tag:        "latest",
		Cmd:        []string{"server", "/data"},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"9000/tcp": {{HostPort: "9000"}},
		},
		Env: []string{fmt.Sprintf("MINIO_ACCESS_KEY=%s", minioAccessKey), fmt.Sprintf("MINIO_SECRET_KEY=%s", minioSecretKey)},
	}

	resource, err := poolMinio.RunWithOptions(options)
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}
	ResourceMinio = resource
	endpoint = fmt.Sprintf("localhost:%s", resource.GetPort("9000/tcp"))
	if err := poolMinio.Retry(func() error {
		url := fmt.Sprintf("http://%s/minio/health/live", endpoint)
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf(err.Error())
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status code not OK")
		}
		return nil
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

}

func TestProcessMessage(t *testing.T) {
	log.Println("[√] start test")
	JeagerStatus = true
	AdpatationReuquestExchange = "adaptation-exchange"
	AdpatationReuquestRoutingKey = "adaptation-request"
	AdpatationReuquestQueueName = "adaptation-request-queue"

	ProcessingRequestExchange = "processing-request-exchange"
	ProcessingRequestRoutingKey = "processing-request"
	ProcessingRequestQueueName = "processing-request"

	ProcessingOutcomeExchange = "processing-outcome-exchange"
	ProcessingOutcomeRoutingKey = "processing-outcome"
	ProcessingOutcomeQueueName = "processing-outcome-queue"

	AdaptationOutcomeExchange = "adaptation-exchange"
	AdaptationOutcomeRoutingKey = "adaptation-exchange"
	AdaptationOutcomeQueueName = "amq.rabbitmq.reply-to"
	// get env secrets
	var errstring error

	secretsstring, errstring = GenerateRandomString(8)
	if errstring != nil {
		log.Fatalf("[x] GenerateRandomString error: %s", errstring)

		return
	}

	minioAccessKey = secretsstring
	minioSecretKey = secretsstring
	if JeagerStatus == true {
		jaegerserver()
		log.Println("[√] create Jaeger  successfully")
		tracer, closer := tracing.Init(thisServiceName)
		defer closer.Close()
		opentracing.SetGlobalTracer(tracer)
		ProcessTracer = tracer
		log.Println("[√] create Jaeger ProcessTracer successfully")

		tracer, closer = tracing.Init("outMsgs")
		defer closer.Close()
		opentracing.SetGlobalTracer(tracer)
		ProcessTracer2 = tracer
		log.Println("[√] create Jaeger ProcessTracer2 successfully")
	}

	rabbitserver()
	log.Println("[√] create AMQP  successfully")

	minioserver()
	log.Println("[√] create minio  successfully")

	time.Sleep(40 * time.Second)

	var err error
	// Get a connrecive //rabbitmq

	connrecive, err = amqp.Dial("amqp://localhost:5672")
	if err != nil {
		log.Fatalf("[x] AMQP connrecive error: %s", err)
	}
	log.Println("[√] AMQP Connected successfully")
	defer connrecive.Close()
	// now we can instantiate minio client
	minioClient, err = min7.New(endpoint, &min7.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalf("[x] Failed to create minio client error: %s", err)
		return
	}
	log.Println("[√] create minio client successfully")
	// Start a consumer
	_, ch, err := rabbitmq.NewQueueConsumer(connrecive, AdpatationReuquestQueueName, AdpatationReuquestExchange, AdpatationReuquestRoutingKey)
	if err != nil {
		log.Fatalf("[x] could not start  AdpatationReuquest consumer error: %s", err)
	}
	log.Println("[√] create start  Adpatation Reuquest consumer successfully")
	defer ch.Close()

	_, outChannel, err := rabbitmq.NewQueueConsumer(connrecive, ProcessingOutcomeQueueName, ProcessingOutcomeExchange, ProcessingOutcomeRoutingKey)
	if err != nil {
		log.Fatalf("[x] Failed to create consumer error: %s", err)

	}
	log.Println("[√] create  consumer successfully")
	defer outChannel.Close()

	sourceMinioBucket = "source"
	cleanMinioBucket = "clean"

	err = createBucketIfNotExist(sourceMinioBucket)
	if err != nil {
		log.Fatalf("[x] sourceMinioBucket createBucketIfNotExist error: %s", err)
	}
	log.Println("[√] create source Minio Bucket successfully")

	err = createBucketIfNotExist(cleanMinioBucket)
	if err != nil {
		log.Fatalf("[x] cleanMinioBucket createBucketIfNotExist error: %s", err)

	}
	log.Println("[√] create clean Minio Bucket successfully")
	fn := "unittest.pdf"
	fullpath := fmt.Sprintf("%s", fn)
	fnrebuild := fmt.Sprintf("rebuild-%s", fn)
	table := amqp.Table{
		"file-id":               fn,
		"source-file-location":  fullpath,
		"rebuilt-file-location": fnrebuild,
		"generate-report":       "true",
		"request-mode":          "respmod",
	}

	var d amqp.Delivery
	d.Headers = table
	t.Run("ProcessMessage", func(t *testing.T) {
		result := processMessage(d)
		if result != nil {

			t.Errorf("processMessage(amqp.Delivery) = %d; want nil", result)

		} else {
			log.Println("[√] ProcessMessage successfully")

		}

	})
	tableout := amqp.Table{
		"file-id":               fn,
		"clean-presigned-url":   "http://localhost:9000",
		"rebuilt-file-location": "./reb.pdf",
		"reply-to":              "replay",
	}

	var dout amqp.Delivery
	dout.Headers = tableout
	t.Run("outcomeProcessMessage", func(t *testing.T) {
		testresult := outcomeProcessMessage(dout)
		if testresult != nil {

			t.Errorf("outcomeProcessMessage(amqp.Delivery) = %d; want nil", testresult)

		} else {
			log.Println("[√] ProcessMessage successfully")

		}
	})
	if JeagerStatus == true {

		t.Run("outcomeProcessMessagewithtrace", func(t *testing.T) {
			helloTo = d.Headers["file-id"].(string)
			span := ProcessTracer.StartSpan("ProcessFile")
			span.SetTag("send-msg", helloTo)
			defer span.Finish()

			ctx = opentracing.ContextWithSpan(context.Background(), span)
			if err := Inject(span, tableout); err != nil {
				t.Errorf("outcomeProcessMessagewithtrace(amqp.Delivery) = %d; want nil", err)

			}
			testresult := outcomeProcessMessage(dout)
			if testresult != nil {

				t.Errorf("outcomeProcessMessage(amqp.Delivery) = %d; want nil", testresult)

			} else {
				log.Println("[√] ProcessMessage successfully")

			}
		})
	}
	// When you're done, kill and remove the container
	if err = poolMq.Purge(ResourceMQ); err != nil {
		fmt.Printf("Could not purge resource: %s", err)
	}
	if err = poolMinio.Purge(ResourceMinio); err != nil {
		fmt.Printf("Could not purge resource: %s", err)
	}
	if JeagerStatus == true {

		if err = poolJG.Purge(ResourceJG); err != nil {
			fmt.Printf("Could not purge resource: %s", err)
		}
	}

}
func GenerateRandomString(n int) (string, error) {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}

func TestInject(t *testing.T) {
	tableout := amqp.Table{
		"file-id":               "id-test",
		"clean-presigned-url":   "http://localhost:9000",
		"rebuilt-file-location": "./reb.pdf",
		"reply-to":              "replay",
	}
	tracer, closer := tracing.Init(thisServiceName)
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)
	ProcessTracer = tracer

	var d amqp.Delivery
	d.Headers = tableout
	helloTo = d.Headers["file-id"].(string)
	span := ProcessTracer.StartSpan("ProcessFile")
	span.SetTag("send-msg", helloTo)
	defer span.Finish()

	ctx = opentracing.ContextWithSpan(context.Background(), span)

	type args struct {
		span opentracing.Span
		hdrs amqp.Table
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			"Inject",
			args{span, tableout},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Inject(tt.args.span, tt.args.hdrs); (err != nil) != tt.wantErr {
				t.Errorf("Inject() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	tableout := amqp.Table{
		"file-id":               "id-test",
		"clean-presigned-url":   "http://localhost:9000",
		"rebuilt-file-location": "./reb.pdf",
		"reply-to":              "replay",
	}
	tracer, closer := tracing.Init(thisServiceName)
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)
	ProcessTracer = tracer

	var d amqp.Delivery
	d.Headers = tableout
	helloTo = d.Headers["file-id"].(string)
	span := ProcessTracer.StartSpan("ProcessFile")
	span.SetTag("send-msg", helloTo)
	defer span.Finish()

	ctx = opentracing.ContextWithSpan(context.Background(), span)
	Inject(span, d.Headers)
	spanctx, _ := Extract(d.Headers)
	type args struct {
		hdrs amqp.Table
	}
	tests := []struct {
		name    string
		args    args
		want    opentracing.SpanContext
		wantErr bool
	}{
		{
			"Extract",
			args{tableout},
			spanctx,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Extract(tt.args.hdrs)
			if (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Extract() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_amqpHeadersCarrier_ForeachKey(t *testing.T) {
	tableout := amqp.Table{
		"file-id":               "id-test",
		"clean-presigned-url":   "http://localhost:9000",
		"rebuilt-file-location": "./reb.pdf",
		"reply-to":              "replay",
	}
	c := amqpHeadersCarrier(tableout)

	tetsc := c

	type args struct {
		handler func(key, val string) error
	}
	han := args{
		handler: func(key string, val string) error {
			return nil
		},
	}

	tests := []struct {
		name    string
		c       amqpHeadersCarrier
		args    args
		wantErr bool
	}{
		{
			"amqpHeadersCarrier",
			tetsc,
			han,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.ForeachKey(tt.args.handler); (err != nil) != tt.wantErr {
				t.Errorf("amqpHeadersCarrier.ForeachKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractWithTracer(t *testing.T) {
	tableout := amqp.Table{
		"file-id":               "id-test",
		"clean-presigned-url":   "http://localhost:9000",
		"rebuilt-file-location": "./reb.pdf",
		"reply-to":              "replay",
	}
	tracer, closer := tracing.Init(thisServiceName)
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)
	ProcessTracer = tracer

	var d amqp.Delivery
	d.Headers = tableout
	helloTo = d.Headers["file-id"].(string)
	span := ProcessTracer.StartSpan("ProcessFile")
	span.SetTag("send-msg", helloTo)
	defer span.Finish()

	ctx = opentracing.ContextWithSpan(context.Background(), span)
	Inject(span, d.Headers)
	spanctx, _ := ExtractWithTracer(d.Headers, ProcessTracer)

	type args struct {
		hdrs   amqp.Table
		tracer opentracing.Tracer
	}
	tests := []struct {
		name    string
		args    args
		want    opentracing.SpanContext
		wantErr bool
	}{
		{
			"ExtractWithTracer",
			args{tableout, ProcessTracer},
			spanctx,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractWithTracer(tt.args.hdrs, tt.args.tracer)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractWithTracer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractWithTracer() = %v, want %v", got, tt.want)
			}
		})
	}
}

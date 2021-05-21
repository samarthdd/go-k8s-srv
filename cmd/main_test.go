package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/k8-proxy/k8-go-comm/pkg/rabbitmq"
	min7 "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/streadway/amqp"
)

// var body = "body test"
var TestMQTable amqp.Table
var endpoint string
var ResourceMQ *dockertest.Resource
var ResourceMinio *dockertest.Resource
var poolMq *dockertest.Pool
var poolMinio *dockertest.Pool

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

	minioAccessKey = os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey = os.Getenv("MINIO_SECRET_KEY")

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
	JeagerStatus = false
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

	minioAccessKey = os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey = os.Getenv("MINIO_SECRET_KEY")
	if minioAccessKey == "" || minioSecretKey == "" {
		log.Fatalf("[x] get Getenv error fail")
		return
	}
	log.Println("[√] get Getenv  successfully")

	rabbitserver()
	log.Println("[√] create AMQP  successfully")

	minioserver()
	log.Println("[√] create minio  successfully")

	time.Sleep(40 * time.Second)

	var err error
	// Get a connection //rabbitmq
	connection, err = amqp.Dial("amqp://localhost:5672")
	if err != nil {
		log.Fatalf("[x] AMQP connection error: %s", err)
	}
	log.Println("[√] AMQP Connected successfully")
	defer connection.Close()
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
	_, ch, err := rabbitmq.NewQueueConsumer(connection, AdpatationReuquestQueueName, AdpatationReuquestExchange, AdpatationReuquestRoutingKey)
	if err != nil {
		log.Fatalf("[x] could not start  AdpatationReuquest consumer error: %s", err)
	}
	log.Println("[√] create start  Adpatation Reuquest consumer successfully")
	defer ch.Close()

	_, outChannel, err := rabbitmq.NewQueueConsumer(connection, ProcessingOutcomeQueueName, ProcessingOutcomeExchange, ProcessingOutcomeRoutingKey)
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
		processMessage(d)

	})
	// When you're done, kill and remove the container
	if err = poolMq.Purge(ResourceMQ); err != nil {
		fmt.Printf("Could not purge resource: %s", err)
	}
	if err = poolMinio.Purge(ResourceMinio); err != nil {
		fmt.Printf("Could not purge resource: %s", err)
	}

}

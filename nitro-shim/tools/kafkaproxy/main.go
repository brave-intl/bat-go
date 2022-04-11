package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"

	utils "nitro-shim/utils"

	msg "github.com/brave-experiments/ia2/message"
	"github.com/segmentio/kafka-go"
)

const (
	envListenAddr  = "KAFKA_PROXY_LISTEN_ADDR"
	envKafkaBroker = "KAFKA_BROKERS"
	envKafkaKey    = "KAFKA_KEY_PATH"
	envKafkaCert   = "KAFKA_CERT_PATH"
	envKafkaTopic  = "KAFKA_TOPIC"

	typeNumReqs = iota
	typeNumGoodFwds
	typeNumBadFwds
)

var l = utils.NewLogger("kafkaproxy: ")

// kafkaProxy represents a proxy that takes as input HTTP requests, turns them
// into Kafka messages, and sends them to a broker.  Think of the proxy as an
// HTTP-to-Kafka bridge.
type kafkaProxy struct {
	writer *kafka.Writer
	stats  *statistics
	addrs  msg.WalletsByKeyID
}

// statistics represents simple statistics of the Kafka proxy.
type statistics struct {
	sync.Mutex
	numReqs     int
	numGoodFwds int
	numBadFwds  int
}

// inc increments the given statistic.
func (s *statistics) inc(statType int) {
	s.Lock()
	defer s.Unlock()

	switch statType {
	case typeNumReqs:
		s.numReqs++
	case typeNumGoodFwds:
		s.numGoodFwds++
	case typeNumBadFwds:
		s.numBadFwds++
	}
}

// get returns the given statistic.
func (s *statistics) get(statType int) int {
	s.Lock()
	defer s.Unlock()

	switch statType {
	case typeNumReqs:
		return s.numReqs
	case typeNumGoodFwds:
		return s.numGoodFwds
	case typeNumBadFwds:
		return s.numBadFwds
	}
	return 0
}

// forwards forwards the currently cached addresses to the Kafka broker.  If
// anything goes wrong, the function returns an error.
func (p *kafkaProxy) forward() error {
	jsonStr, err := json.Marshal(p.addrs)
	if err != nil {
		return err
	}

	err = p.writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   nil,
			Value: []byte(jsonStr),
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// getAddressesHandler returns an HTTP handler that answers /addresses
// requests, i.e., submissions of freshly anonymizes IP addresses.
func getAddressesHandler(p *kafkaProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.stats.inc(typeNumReqs)
		if r.Method != http.MethodPost {
			http.Error(w, "only POST requests are accepted", http.StatusBadRequest)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var addrs = make(msg.WalletsByKeyID)
		if err := addrs.UnmarshalJSON(body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := p.forward(); err != nil {
			http.Error(w, fmt.Sprintf("failed to forward addresses: %s", err),
				http.StatusInternalServerError)
			p.stats.inc(typeNumBadFwds)
			return
		}
		p.stats.inc(typeNumGoodFwds)
	}
}

// getStatusHandler returns an HTTP handler that answers /status requests,
// i.e., requests for the proxy's statistics.
func getStatusHandler(p *kafkaProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "# of forward requests: %d\n"+
			"# of successful forwards: %d\n"+
			"# of failed forwards: %d\n",
			p.stats.get(typeNumReqs),
			p.stats.get(typeNumGoodFwds),
			p.stats.get(typeNumBadFwds))
	}
}

// newKafkaProxy creates a new Kafka proxy.
func newKafkaProxy(certFile, keyFile, topic string) (*kafkaProxy, error) {
	p := &kafkaProxy{
		stats: new(statistics),
		addrs: make(msg.WalletsByKeyID),
	}

	kafkaBroker, exists := os.LookupEnv(envKafkaBroker)
	if !exists {
		return nil, fmt.Errorf("environment variable %q not set", envKafkaBroker)
	}
	if kafkaBroker == "" {
		return nil, fmt.Errorf("environment variable %q empty", envKafkaBroker)
	}
	l.Printf("Fetched Kafka broker %q from environment variable.", kafkaBroker)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	l.Println("Loaded certificate and key file for Kafka.")

	p.writer = &kafka.Writer{
		Addr:  kafka.TCP(kafkaBroker),
		Topic: topic,
		Transport: &kafka.Transport{
			TLS: &tls.Config{Certificates: []tls.Certificate{cert}},
		},
	}

	return p, nil
}

func main() {
	var err error
	var proxy *kafkaProxy

	var cfg = map[string]string{
		envListenAddr:  "",
		envKafkaBroker: "",
		envKafkaKey:    "",
		envKafkaCert:   "",
		envKafkaTopic:  "",
	}
	if err := utils.ReadConfigFromEnv(cfg); err != nil {
		l.Fatalf("Failed to read config from environment variables: %s", err)
	}

	if proxy, err = newKafkaProxy(cfg[envKafkaCert], cfg[envKafkaKey], cfg[envKafkaTopic]); err != nil {
		l.Fatalf("Failed to create Kafka writer: %s", err)
	}

	http.HandleFunc("/addresses", getAddressesHandler(proxy))
	http.HandleFunc("/status", getStatusHandler(proxy))
	l.Println("Starting Kafka proxy.")
	l.Fatal(http.ListenAndServe(cfg[envListenAddr], nil))
}

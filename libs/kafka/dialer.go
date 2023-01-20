package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/aws_msk_iam_v2"

	"github.com/brave-intl/bat-go/libs/aws"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
)

// Reader - implements KafkaReader
type Reader struct {
	kafkaReader *kafka.Reader
	kafkaDialer *kafka.Dialer
}

// NewKafkaReader - creates a new kafka reader for groupID and topic
func NewKafkaReader(ctx context.Context, groupID string, topic string) (*Reader, error) {
	logger := logging.Logger(ctx, "kafka.NewKafkaReader")

	var (
		dialer   *kafka.Dialer
		err      error
		x509Cert *x509.Certificate
	)
	if saslEnabled, ok := ctx.Value(appctx.MSKSASLEnabledCTXKey).(bool); ok && saslEnabled {
		cfg, err := aws.BaseAWSConfig(ctx, logger)
		if err != nil {
			return nil, fmt.Errorf("kafka reader: could not create aws config: %w", err)
		}
		// sasl mechanism for dialer
		mechanism := aws_msk_iam_v2.NewMechanism(cfg)

		dialer = &kafka.Dialer{
			Timeout:       10 * time.Second,
			DualStack:     true,
			SASLMechanism: mechanism,
			TLS:           &tls.Config{},
		}
	} else {
		dialer, x509Cert, err = TLSDialer()
		if err != nil {
			return nil, fmt.Errorf("kafka reader: could not create new kafka reader: %w", err)
		}
		ctx = context.WithValue(ctx, appctx.Kafka509CertCTXKey, x509Cert)
	}

	// throw the cert on the context, instrument kafka
	InstrumentKafka(ctx)

	kafkaBrokers := ctx.Value(appctx.KafkaBrokersCTXKey).(string)

	kafkaReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:       strings.Split(kafkaBrokers, ","),
		RetentionTime: 10080 * time.Minute, // 7 Days
		GroupID:       groupID,
		Topic:         topic,
		Dialer:        dialer,
		Logger:        kafka.LoggerFunc(logger.Printf), // FIXME
	})

	return &Reader{
		kafkaReader: kafkaReader,
		kafkaDialer: dialer,
	}, nil
}

// ReadMessage - reads kafka messages
func (k *Reader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	return k.kafkaReader.ReadMessage(ctx)
}

// TLSDialer creates a Kafka dialer over TLS. The function requires
// KAFKA_SSL_CERTIFICATE_LOCATION and KAFKA_SSL_KEY_LOCATION environment
// variables to be set.
func TLSDialer() (*kafka.Dialer, *x509.Certificate, error) {

	caPEM, err := readFileFromEnvLoc("KAFKA_SSL_CA_LOCATION", false)
	if err != nil {
		return nil, nil, err
	}

	certEnv := "KAFKA_SSL_CERTIFICATE"
	certPEM := []byte(os.Getenv(certEnv))
	if len(certPEM) == 0 {
		certPEM, err = readFileFromEnvLoc("KAFKA_SSL_CERTIFICATE_LOCATION", true)
		if err != nil {
			return nil, nil, err
		}
	}

	keyEnv := "KAFKA_SSL_KEY"
	encryptedKeyPEM := []byte(os.Getenv(keyEnv))

	// Check to see if KAFKA_SSL_CERTIFICATE includes both certificate and key
	if certPEM[0] == '{' {
		type Certificate struct {
			Certificate string `json:"certificate"`
			Key         string `json:"key"`
		}
		var cert Certificate
		err := json.Unmarshal(certPEM, &cert)
		if err != nil {
			return nil, nil, err
		}
		certPEM = []byte(cert.Certificate)
		encryptedKeyPEM = []byte(cert.Key)
	}

	if len(encryptedKeyPEM) == 0 {
		encryptedKeyPEM, err = readFileFromEnvLoc("KAFKA_SSL_KEY_LOCATION", true)
		if err != nil {
			return nil, nil, err
		}
	}

	block, rest := pem.Decode(encryptedKeyPEM)
	if len(rest) > 0 {
		return nil, nil, errors.New("extra data in KAFKA_SSL_KEY")
	}

	keyPEM := pem.EncodeToMemory(block)

	certificate, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "Could not parse x509 keypair")
	}

	// Define TLS configuration
	config := &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}

	// Instrument kafka cert expiration information
	x509Cert, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "Could not parse certificate")
	}

	if time.Now().After(x509Cert.NotAfter) {
		// the certificate has expired, raise error
		return nil, nil, errorutils.ErrCertificateExpired
	}

	if len(caPEM) > 0 {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(caPEM)); !ok {
			return nil, nil, errors.New("could not add custom CA from KAFKA_SSL_CA_LOCATION")
		}
		config.RootCAs = caCertPool
	}

	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		TLS:       config}

	return dialer, x509Cert, nil
}

func readFileFromEnvLoc(env string, required bool) ([]byte, error) {
	loc := os.Getenv(env)
	if len(loc) == 0 {
		if !required {
			return []byte{}, nil
		}
		return []byte{}, errors.New(env + " must be passed")
	}
	buf, err := ioutil.ReadFile(loc)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

// InitKafkaWriter - create a kafka writer given a topic
func InitKafkaWriter(ctx context.Context, topic string) (*kafka.Writer, *kafka.Dialer, error) {
	logger := logging.Logger(ctx, "kafka.InitKafkaWriter")

	dialer, x509Cert, err := TLSDialer()
	if err != nil {
		return nil, nil, err
	}

	// throw the cert on the context, instrument kafka
	InstrumentKafka(context.WithValue(ctx, appctx.Kafka509CertCTXKey, x509Cert))

	kafkaBrokers := ctx.Value(appctx.KafkaBrokersCTXKey).(string)

	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  strings.Split(kafkaBrokers, ","),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
		Dialer:   dialer,
		Logger:   kafka.LoggerFunc(logger.Printf), // FIXME
	})

	return kafkaWriter, dialer, nil
}

// GenerateCodecs - create a map of codec name to the avro codec
func GenerateCodecs(codecs map[string]string) (map[string]*goavro.Codec, error) {
	var (
		res = make(map[string]*goavro.Codec)
		err error
	)
	for k, v := range codecs {
		res[k], err = goavro.NewCodec(v)
		if err != nil {
			return nil, fmt.Errorf("failed to generate codec: %w", err)
		}
	}
	return res, nil
}

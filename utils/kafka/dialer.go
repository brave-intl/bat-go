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
	"time"

	"github.com/linkedin/goavro"
	kafka "github.com/segmentio/kafka-go"

	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	stringutils "github.com/brave-intl/bat-go/utils/string"
)

var (
	groupID = fmt.Sprintf("%s.%s", os.Getenv("ENV"), os.Getenv("SERVICE"))
)

// TLSDialer creates a Kafka dialer over TLS. The function requires
// KAFKA_SSL_CERTIFICATE_LOCATION and KAFKA_SSL_KEY_LOCATION environment
// variables to be set.
func TLSDialer() (*kafka.Dialer, *x509.Certificate, error) {
	keyPasswordEnv := "KAFKA_SSL_KEY_PASSWORD"
	keyPassword := os.Getenv(keyPasswordEnv)

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
	if len(keyPassword) != 0 {
		// TODO: move away from DecryptPEM in 1.16
		keyDER, err := x509.DecryptPEMBlock(block, []byte(keyPassword)) //nolint
		if err != nil {
			return nil, nil, errorutils.Wrap(err, "decrypt KAFKA_SSL_KEY failed")
		}

		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	}

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

// InitDialer creates a dialer to be used by kafka
func InitDialer(
	ctx context.Context,
	dialers ...*kafka.Dialer,
) (*kafka.Dialer, error) {
	var dialer *kafka.Dialer
	if len(dialers) != 0 && dialers[0] != nil {
		return dialers[0], nil
	}

	dialer, x509Cert, err := TLSDialer()
	if err != nil {
		return nil, err
	}

	// throw the cert on the context, instrument kafka
	InstrumentKafka(context.WithValue(ctx, appctx.Kafka509CertCTXKey, x509Cert))
	return dialer, err
}

// InitKafkaWriter - create a kafka writer given a topic
func InitKafkaWriter(
	ctx context.Context,
	topic string,
	dialers ...*kafka.Dialer,
) (*kafka.Writer, *kafka.Dialer, error) {
	_, logger := logging.SetupLogger(ctx)

	dialer, err := InitDialer(ctx, dialers...)
	if err != nil {
		return nil, nil, err
	}

	kafkaBrokers := Brokers(ctx)

	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  kafkaBrokers,
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
		Dialer:   dialer,
		Logger:   kafka.LoggerFunc(logger.Printf), // FIXME
	})

	return kafkaWriter, dialer, nil
}

// Brokers returns the list of brokers from a context
func Brokers(ctx context.Context) []string {
	return stringutils.SplitAndTrim(ctx.Value(appctx.KafkaBrokersCTXKey).(string))
}

// InitKafkaReader - create a kafka writer given a topic
func InitKafkaReader(
	ctx context.Context,
	topic string,
	dialers ...*kafka.Dialer,
) (*kafka.Reader, *kafka.Dialer, error) {
	_, logger := logging.SetupLogger(ctx)

	dialer, err := InitDialer(ctx, dialers...)
	if err != nil {
		return nil, nil, err
	}

	kafkaBrokers := Brokers(ctx)

	kafkaReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     kafkaBrokers,
		Topic:       topic,
		Dialer:      dialer,
		MaxWait:     time.Second * 5,
		GroupID:     groupID,
		StartOffset: kafka.FirstOffset,
		Logger:      kafka.LoggerFunc(logger.Printf), // FIXME
	})

	return kafkaReader, dialer, TryKafkaConnection(dialer, kafkaBrokers)
}

// TryKafkaConnection tries connecting to list of brokers. If at least one
// broker can be reached and connection is successful, error is nil.
// Otherwise, it returns an error describing all connection errors.
func TryKafkaConnection(dialer *kafka.Dialer, brokers []string) error {
	var errors []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, broker := range brokers {
		conn, err := dialer.DialContext(ctx, "tcp", broker)
		if err != nil {
			errors = append(errors, err.Error())
		} else {
			// at least one successful broker connection
			return conn.Close()
		}
	}
	return fmt.Errorf("%s", errors)
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

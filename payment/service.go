package payment

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	wallet "github.com/brave-intl/bat-go/wallet/service"
	"github.com/linkedin/goavro"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

var voteTopic = os.Getenv("ENV") + ".payment.vote"
var kafkaCertNotBefore = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "kafka_cert_not_before",
	Help: "Date when the kafka certificate becomes valid.",
})
var kafkaCertNotAfter = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "kafka_cert_not_after",
	Help: "Date when the kafka certificate expires.",
})

func init() {
	prometheus.MustRegister(kafkaCertNotBefore)
	prometheus.MustRegister(kafkaCertNotAfter)
}

// Service contains datastore
type Service struct {
	wallet      wallet.Service
	cbClient    cbr.Client
	datastore   Datastore
	codecs      map[string]*goavro.Codec
	kafkaWriter *kafka.Writer
	kafkaDialer *kafka.Dialer
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

func tlsDialer() (*kafka.Dialer, error) {
	keyPasswordEnv := "KAFKA_SSL_KEY_PASSWORD"
	keyPassword := os.Getenv(keyPasswordEnv)

	caPEM, err := readFileFromEnvLoc("KAFKA_SSL_CA_LOCATION", false)
	if err != nil {
		return nil, err
	}

	certEnv := "KAFKA_SSL_CERTIFICATE"
	certPEM := []byte(os.Getenv(certEnv))
	if len(certPEM) == 0 {
		certPEM, err = readFileFromEnvLoc("KAFKA_SSL_CERTIFICATE_LOCATION", true)
		if err != nil {
			return nil, err
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
			return nil, err
		}
		certPEM = []byte(cert.Certificate)
		encryptedKeyPEM = []byte(cert.Key)
	}

	if len(encryptedKeyPEM) == 0 {
		encryptedKeyPEM, err = readFileFromEnvLoc("KAFKA_SSL_KEY_LOCATION", true)
		if err != nil {
			return nil, err
		}
	}

	block, rest := pem.Decode(encryptedKeyPEM)
	if len(rest) > 0 {
		return nil, errors.New("Extra data in KAFKA_SSL_KEY")
	}

	keyPEM := pem.EncodeToMemory(block)
	if len(keyPassword) != 0 {
		keyDER, err := x509.DecryptPEMBlock(block, []byte(keyPassword))
		if err != nil {
			return nil, errors.Wrap(err, "decrypt KAFKA_SSL_KEY failed")
		}

		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	}

	certificate, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return nil, errors.Wrap(err, "Could not parse x509 keypair")
	}

	// Define TLS configuration
	config := &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}

	// Instrument kafka cert expiration information
	x509Cert, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, errors.Wrap(err, "Could not parse certificate")
	}
	kafkaCertNotBefore.Set(float64(x509Cert.NotBefore.Unix()))
	kafkaCertNotAfter.Set(float64(x509Cert.NotAfter.Unix()))

	if len(caPEM) > 0 {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(caPEM)); !ok {
			return nil, errors.New("Could not add custom CA from KAFKA_SSL_CA_LOCATION")
		}
		config.RootCAs = caCertPool
	}

	return &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		TLS:       config}, nil
}

// InitCodecs used for Avro encoding / decoding
func (service *Service) InitCodecs() error {
	service.codecs = make(map[string]*goavro.Codec)

	voteEventCodec, err := goavro.NewCodec(string(voteSchema))
	service.codecs["vote"] = voteEventCodec
	if err != nil {
		return err
	}
	return nil
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (service *Service) InitKafka() error {
	dialer, err := tlsDialer()
	if err != nil {
		return err
	}
	service.kafkaDialer = dialer

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:  strings.Split(kafkaBrokers, ","),
		Topic:    voteTopic,
		Balancer: &kafka.LeastBytes{},
		Dialer:   dialer,
		Logger:   kafka.LoggerFunc(log.Printf), // FIXME
	})

	service.kafkaWriter = kafkaWriter
	err = service.InitCodecs()
	if err != nil {
		return err
	}

	return nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore) (*Service, error) {
	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	walletService, err := wallet.InitService(datastore, nil)
	if err != nil {
		return nil, err
	}

	service := &Service{
		wallet:    *walletService,
		cbClient:  cbClient,
		datastore: datastore,
	}
	err = service.InitKafka()
	if err != nil {
		return nil, err
	}
	return service, nil
}

// CreateOrderFromRequest creates an order from the request
func (service *Service) CreateOrderFromRequest(req CreateOrderRequest) (*Order, error) {
	totalPrice := decimal.New(0, 0)
	orderItems := []OrderItem{}
	var currency string

	for i := 0; i < len(req.Items); i++ {
		orderItem, err := createOrderItemFromMacaroon(req.Items[i].SKU, req.Items[i].Quantity)
		if err != nil {
			return nil, err
		}
		totalPrice = totalPrice.Add(orderItem.Subtotal)

		if currency == "" {
			currency = orderItem.Currency
		}
		if currency != orderItem.Currency {
			return nil, errors.New("all order items must be the same currency")
		}
		orderItems = append(orderItems, *orderItem)
	}

	order, err := service.datastore.CreateOrder(totalPrice, "brave.com", "pending", currency, orderItems)

	return order, err
}

// UpdateOrderStatus checks to see if an order has been paid and updates it if so
func (service *Service) UpdateOrderStatus(orderID uuid.UUID) error {
	order, err := service.datastore.GetOrder(orderID)
	if err != nil {
		return err
	}

	sum, err := service.datastore.GetSumForTransactions(orderID)
	if err != nil {
		return err
	}

	if sum.GreaterThanOrEqual(order.TotalPrice) {
		err = service.datastore.UpdateOrder(orderID, "paid")
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateTransactionFromRequest queries the endpoints and creates a transaciton
func (service *Service) CreateTransactionFromRequest(req CreateTransactionRequest, orderID uuid.UUID) (*Transaction, error) {
	var wallet uphold.Wallet
	upholdTransaction, err := wallet.GetTransaction(req.ExternalTransactionID)

	if err != nil {
		return nil, err
	}

	amount := upholdTransaction.AltCurrency.FromProbi(upholdTransaction.Probi)
	status := upholdTransaction.Status
	currency := upholdTransaction.AltCurrency.String()
	kind := "uphold"

	transaction, err := service.datastore.CreateTransaction(orderID, req.ExternalTransactionID, status, currency, kind, amount)
	if err != nil {
		return nil, errors.Wrap(err, "Error recording transaction")
	}

	isPaid, err := service.IsOrderPaid(transaction.OrderID)
	if err != nil {
		return nil, errors.Wrap(err, "Error submitting anon card transaction")
	}

	// If the transaction that was satisifies the order then let's update the status
	if isPaid {
		err = service.datastore.UpdateOrder(transaction.OrderID, "paid")
		if err != nil {
			return nil, errors.Wrap(err, "Error updating order status")
		}
	}

	return transaction, err
}

// CreateAnonCardTransaction takes a signed transaction and executes it on behalf of an anon card
func (service *Service) CreateAnonCardTransaction(ctx context.Context, walletID uuid.UUID, transaction string, orderID uuid.UUID) (*Transaction, error) {
	txInfo, err := service.wallet.SubmitAnonCardTransaction(ctx, walletID, transaction)
	if err != nil {
		return nil, errors.Wrap(err, "Error submitting anon card transaction")
	}
	fmt.Println(txInfo)

	txn, err := service.datastore.CreateTransaction(orderID, txInfo.ID, txInfo.Status, txInfo.DestCurrency, "anonymous-card", txInfo.DestAmount)
	if err != nil {
		return nil, errors.Wrap(err, "Error recording anon card transaction")
	}

	err = service.UpdateOrderStatus(orderID)
	if err != nil {
		return nil, errors.Wrap(err, "Error updating order status")
	}

	return txn, err
}

// IsOrderPaid determines if the order has been paid
func (service *Service) IsOrderPaid(orderID uuid.UUID) (bool, error) {
	// Now that the transaction has been created let's check to see if that fulfilled the order.
	order, err := service.datastore.GetOrder(orderID)
	if err != nil {
		return false, err
	}

	sum, err := service.datastore.GetSumForTransactions(orderID)
	if err != nil {
		return false, err
	}

	return sum.GreaterThanOrEqual(order.TotalPrice), nil
}

// RunNextOrderJob takes the next order job and completes it
func (service *Service) RunNextOrderJob(ctx context.Context) (bool, error) {
	return service.datastore.RunNextOrderJob(ctx, service)
}

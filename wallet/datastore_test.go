// +build integration

package wallet

import (
	"testing"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type PostgresTestSuite struct {
	suite.Suite
}

func TestPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresTestSuite))
}

func (suite *PostgresTestSuite) SetupSuite() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	m, err := pg.NewMigrate()
	suite.Require().NoError(err, "Failed to create migrate instance")

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}
	if ver > 0 {
		suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	}

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
}

func (suite *PostgresTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *PostgresTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *PostgresTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table + " returning *")
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *PostgresTestSuite) TestInsertWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

	wallet := &Info{ID: uuid.NewV4().String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Assert().NoError(pg.InsertWallet(wallet), "Save wallet should succeed")
}

func (suite *PostgresTestSuite) TestGetWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err)

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	id := uuid.NewV4()

	origWallet := &Info{ID: id.String(), Provider: "uphold", ProviderID: uuid.NewV4().String(), PublicKey: publicKey}
	suite.Assert().NoError(pg.InsertWallet(origWallet), "Save wallet should succeed")

	wallet, err := pg.GetWallet(id)
	suite.Assert().NoError(err, "Get wallet should succeed")
	suite.Assert().Equal(origWallet, wallet)
}

package oracle_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/oracle"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "oracle",
		Params:          engine.Params{"host": "oracle.example.com", "port": "1521", "serviceName": "ORCLPDB1", "username": "system"},
		Secrets:         engine.Secrets{"password": "ora@secret"},
		LegacyTLSDSN:    "oracle://system:secret@oracle.example.com:1521/ORCLPDB1?SSL=enable",
		ExpectedTLSMode: "enable",
	})
}

package engine

import (
	log "github.com/sirupsen/logrus"
	"math/big"
	"testing"
)

const (
	maxUint = ^uint(0)
	maxInt  = int64(maxUint >> 1)
)

func TestMinMax(t *testing.T) {

	if min(big.NewInt(0), big.NewInt(0)).Cmp(big.NewInt(0)) != 0 {
		log.Println("1")
		t.FailNow()
	}
	if min(big.NewInt(maxInt), big.NewInt(0)).Cmp(big.NewInt(0)) != 0 {
		log.Println("2")
		t.FailNow()
	}
	if min(big.NewInt(maxInt), big.NewInt(1234567890)).Cmp(big.NewInt(1234567890)) != 0 {
		log.Println("3")
		t.FailNow()
	}
	if min(big.NewInt(0), big.NewInt(1234567890)).Cmp(big.NewInt(0)) != 0 {
		log.Println("4")
		t.FailNow()
	}
}

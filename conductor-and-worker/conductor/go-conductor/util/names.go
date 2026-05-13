package util

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"math/big"
	"strings"
)

// Generated Workers    Collision Chance        Rough Odds
// -------------------------------------------------------
// 1,000                ~0.0000000000000217%    ~1 in 4.6 quadrillion
// 10,000               ~0.00000000000217%      ~1 in 46 trillion
// 100,000              ~0.000000000217%        ~1 in 461 billion
// 1,000,000            ~0.0000000217%          ~1 in 4.6 billion
// 10,000,000           ~0.00000217%            ~1 in 46 million
// 100,000,000          ~0.000217%              ~1 in 461,000

const (
	randChars    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
	suffixLength = 10
)

//go:embed names.txt
var namesTxt string

func GenerateWorkerName() string {
	namesSlice := strings.Fields(namesTxt)

	if len(namesSlice) == 0 {
		panic("names.txt is empty")
	}

	nameIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(namesSlice))))
	if err != nil {
		panic(err)
	}

	var nameSuffix [suffixLength]byte
	for nameSuffixIdx := range nameSuffix {
		nameSuffixCharIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(randChars))))
		if err != nil {
			panic(err)
		}

		nameSuffix[nameSuffixIdx] = randChars[nameSuffixCharIdx.Int64()]
	}

	return fmt.Sprintf("%s-%s", namesSlice[nameIdx.Int64()], string(nameSuffix[:]))
}

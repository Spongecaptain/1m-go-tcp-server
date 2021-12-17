package main

import (
	"crypto/sha256"
	"math/big"
	"strconv"
)

// 计算密集型：挖矿算法，计算哈希值，如果哈希值的前 12bit 都是 0 的话算挖矿成功
func pow(targetBits int) *big.Int {
	target := big.NewInt(1)
	target.Lsh(target, uint(256-targetBits))
	var hashInt big.Int
	var hash [32]byte
	nonce := 0

	for {
		data := "hello world " + strconv.Itoa(nonce)
		hash = sha256.Sum256([]byte(data))
		hashInt.SetBytes(hash[:])

		if hashInt.Cmp(target) == -1 {
			break
		} else {
			nonce++
		}
	}

	return &hashInt
}

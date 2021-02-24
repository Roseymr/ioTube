// Copyright (c) 2020 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package witness

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	"github.com/iotexproject/ioTube/witness-service/dispatcher"
)

type service struct {
	cashiers        []TokenCashier
	processor       dispatcher.Runner
	batchSize       uint16
	processInterval time.Duration
	privateKey      *ecdsa.PrivateKey
	witnessAddress  common.Address
}

// NewService creates a new witness service
func NewService(
	privateKey *ecdsa.PrivateKey,
	cashiers []TokenCashier,
	batchSize uint16,
	processInterval time.Duration,
) (Service, error) {
	s := &service{
		cashiers:        cashiers,
		processInterval: processInterval,
		batchSize:       batchSize,
		privateKey:      privateKey,
		witnessAddress:  crypto.PubkeyToAddress(privateKey.PublicKey),
	}
	var err error
	if s.processor, err = dispatcher.NewRunner(processInterval, s.process); err != nil {
		return nil, errors.New("failed to create swapper")
	}

	return s, nil
}

func (s *service) Start(ctx context.Context) error {
	for _, cashier := range s.cashiers {
		if err := cashier.Start(ctx); err != nil {
			return errors.Wrap(err, "failed to start recorder")
		}
	}
	return s.processor.Start()
}

func (s *service) Stop(ctx context.Context) error {
	if err := s.processor.Close(); err != nil {
		return err
	}
	for _, cashier := range s.cashiers {
		if err := cashier.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) sign(transfer *Transfer, validatorContractAddr common.Address) (common.Hash, common.Address, []byte, error) {
	id := crypto.Keccak256Hash(
		validatorContractAddr.Bytes(),
		transfer.cashier.Bytes(),
		transfer.coToken.Bytes(),
		math.U256Bytes(new(big.Int).SetUint64(transfer.index)),
		transfer.sender.Bytes(),
		transfer.recipient.Bytes(),
		math.U256Bytes(transfer.amount),
	)
	signature, err := crypto.Sign(id.Bytes(), s.privateKey)

	return id, s.witnessAddress, signature, err
}

func (s *service) process() error {
	for _, cashier := range s.cashiers {
		if err := cashier.PullTransfers(s.batchSize); err != nil {
			return err
		}
		if err := cashier.SubmitTransfers(s.sign); err != nil {
			return err
		}
		if err := cashier.CheckTransfers(); err != nil {
			return err
		}
	}
	return nil
}

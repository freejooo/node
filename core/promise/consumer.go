/*
 * Copyright (C) 2017 The "MysteriumNetwork/node" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package promise

import (
	"context"
	"encoding/json"
	"fmt"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mysteriumnetwork/node/communication"
	"github.com/mysteriumnetwork/node/core/promise/storage"
	"github.com/mysteriumnetwork/node/identity"
	"github.com/mysteriumnetwork/node/service_discovery/dto"
)

var (
	errLowAmount          = fmt.Errorf("promise amount less then the service proposal price")
	errLowBalance         = fmt.Errorf("issuer balance less then the promise amount")
	errBadSignature       = fmt.Errorf("invalid Signature for the provided identity")
	errUnknownBenefiter   = fmt.Errorf("unknown promise benefiter received")
	errUnsupportedRequest = fmt.Errorf("unsupported request")
)

// Consumer process promise-requests
type Consumer struct {
	proposal    dto.ServiceProposal
	etherClient ethereum.ChainStateReader
	storage     storage.Storage
}

// GetRequestEndpoint returns endpoint where to receive requests
func (c *Consumer) GetRequestEndpoint() communication.RequestEndpoint {
	return endpoint
}

// NewRequest creates struct where request from endpoint will be serialized
func (c *Consumer) NewRequest() (requestPtr interface{}) {
	return &Request{}
}

// Consume handles requests from endpoint and replies with response
func (c *Consumer) Consume(requestPtr interface{}) (response interface{}, err error) {
	request, ok := requestPtr.(*Request)
	if !ok {
		return nil, errUnsupportedRequest
	}

	receivedPromise, err := json.Marshal(request.SignedPromise.Promise)
	if err != nil {
		return nil, err
	}

	signature := identity.SignatureBase64(string(request.SignedPromise.IssuerSignature))
	issuer := identity.FromAddress(request.SignedPromise.Promise.IssuerID)
	verify := identity.NewVerifierIdentity(issuer)
	if !verify.Verify(receivedPromise, signature) {
		return &Response{
			Success: false,
			Message: errBadSignature.Error(),
			Request: request,
		}, nil
	}

	benefiter := identity.FromAddress(request.SignedPromise.Promise.BenefiterID)
	if benefiter.Address != c.proposal.ProviderID {
		return &Response{
			Success: false,
			Message: errUnknownBenefiter.Error(),
			Request: request,
		}, nil
	}

	price := c.proposal.PaymentMethod.GetPrice()
	amount := request.SignedPromise.Promise.Amount.Amount
	if amount < price.Amount {
		return &Response{
			Success: false,
			Message: errLowAmount.Error(),
			Request: request,
		}, nil
	}

	balance, err := c.etherClient.BalanceAt(context.Background(), common.HexToAddress(issuer.Address), nil)
	if err != nil {
		return nil, err
	} else if balance.Uint64() < amount {
		return &Response{
			Success: false,
			Message: errLowBalance.Error(),
			Request: request,
		}, nil
	}

	if err := c.storage.Store(issuer.Address, &request.SignedPromise.Promise); err != nil {
		return nil, err
	}

	return &Response{Success: true, Message: "Promise accepted", Request: request}, nil
}

// Package client provides gRPC client wrappers
package client

import (
	"context"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// PrepareOrderRequest gRPC PrepareOrder request
type PrepareOrderRequest struct {
	Wallet      string
	Market      string
	Side        string
	OrderType   string
	Price       string
	Amount      string
	TimeInForce string
}

// PrepareOrderResponse gRPC PrepareOrder response
type PrepareOrderResponse struct {
	OrderID   string
	Nonce     uint64
	ExpiresAt int64
	TypedData *EIP712TypedData
}

// EIP712TypedData EIP-712 typed data for signing
type EIP712TypedData struct {
	Types       map[string][]EIP712Type `json:"types"`
	PrimaryType string                  `json:"primaryType"`
	Domain      EIP712Domain            `json:"domain"`
	Message     map[string]interface{}  `json:"message"`
}

// EIP712Type EIP-712 type definition
type EIP712Type struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// EIP712Domain EIP-712 domain separator
type EIP712Domain struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	ChainID           int64  `json:"chainId"`
	VerifyingContract string `json:"verifyingContract"`
}

// PrepareOrder calls eidos-trading PrepareOrder gRPC
func (c *TradingClient) PrepareOrder(ctx context.Context, req *PrepareOrderRequest) (*PrepareOrderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	pbReq := &pb.PrepareOrderRequest{
		Wallet:      req.Wallet,
		Market:      req.Market,
		Side:        convertOrderSide(req.Side),
		Type:        convertOrderType(req.OrderType),
		Price:       req.Price,
		Amount:      req.Amount,
		TimeInForce: convertTimeInForce(req.TimeInForce),
	}

	resp, err := c.client.PrepareOrder(ctx, pbReq)
	if err != nil {
		return nil, convertGRPCError(err)
	}

	// Convert typed data from proto
	typedData := convertProtoTypedData(resp.TypedData)

	return &PrepareOrderResponse{
		OrderID:   resp.OrderId,
		Nonce:     resp.Nonce,
		ExpiresAt: resp.ExpiresAt,
		TypedData: typedData,
	}, nil
}

// convertProtoTypedData converts proto TypedData to our struct
func convertProtoTypedData(pd *pb.EIP712TypedData) *EIP712TypedData {
	if pd == nil {
		return nil
	}

	// Convert types
	types := make(map[string][]EIP712Type)
	for name, typeList := range pd.Types {
		eipTypes := make([]EIP712Type, len(typeList.Types))
		for i, t := range typeList.Types {
			eipTypes[i] = EIP712Type{
				Name: t.Name,
				Type: t.Type,
			}
		}
		types[name] = eipTypes
	}

	// Convert domain
	domain := EIP712Domain{}
	if pd.Domain != nil {
		domain.Name = pd.Domain.Name
		domain.Version = pd.Domain.Version
		domain.ChainID = pd.Domain.ChainId
		domain.VerifyingContract = pd.Domain.VerifyingContract
	}

	// Convert message
	message := make(map[string]interface{})
	for k, v := range pd.Message {
		message[k] = v
	}

	return &EIP712TypedData{
		Types:       types,
		PrimaryType: pd.PrimaryType,
		Domain:      domain,
		Message:     message,
	}
}

// Add PrepareOrder to interface
func init() {
	// This file extends TradingClient with PrepareOrder capability
}

// PrepareOrderDTO converts PrepareOrderResponse to DTO
func (r *PrepareOrderResponse) ToDTO() *dto.PrepareOrderResponse {
	var typedData interface{}
	if r.TypedData != nil {
		typedData = r.TypedData
	}

	return &dto.PrepareOrderResponse{
		OrderID:   r.OrderID,
		Nonce:     r.Nonce,
		ExpiresAt: r.ExpiresAt,
		TypedData: typedData,
	}
}

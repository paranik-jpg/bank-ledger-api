package service

import (
	"context"
	"testing"
)

func TestTransferServiceValidation(t *testing.T) {
	// A mock or nil db can test pure business validation without executing SQL
	svc := &TransferService{db: nil}

	tests := []struct {
		name    string
		dto     TransferDTO
		wantErr error
	}{
		{
			name: "Non-positive amount",
			dto: TransferDTO{
				FromAccountID: 1,
				ToAccountID:   2,
				Amount:        -50,
			},
			wantErr: ErrInvalidTransferAmount,
		},
		{
			name: "Zero amount",
			dto: TransferDTO{
				FromAccountID: 1,
				ToAccountID:   2,
				Amount:        0,
			},
			wantErr: ErrInvalidTransferAmount,
		},
		{
			name: "Identical source and destination",
			dto: TransferDTO{
				FromAccountID: 5,
				ToAccountID:   5,
				Amount:        100,
			},
			wantErr: ErrIdenticalAccounts,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ExecuteTransfer(context.Background(), tt.dto)
			if err != tt.wantErr {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

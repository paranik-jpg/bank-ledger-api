package service

import (
	"context"
	"testing"
)

func TestTransferServiceValidation(t *testing.T) {
	// A nil DB is safe here because invalid requests must fail before database access.
	svc := &TransferService{db: nil}

	tests := []struct {
		name string
		dto  TransferDTO
		want error
	}{
		{
			name: "Non-positive amount",
			dto: TransferDTO{
				FromAccountID: 1,
				ToAccountID:   2,
				Amount:        -50,
			},
			want: ErrInvalidTransferAmount,
		},
		{
			name: "Zero amount",
			dto: TransferDTO{
				FromAccountID: 1,
				ToAccountID:   2,
				Amount:        0,
			},
			want: ErrInvalidTransferAmount,
		},
		{
			name: "Identical source and destination",
			dto: TransferDTO{
				FromAccountID: 5,
				ToAccountID:   5,
				Amount:        100,
			},
			want: ErrIdenticalAccounts,
		},
		{
			name: "Identical accounts with positive amount",
			dto: TransferDTO{
				FromAccountID: 5,
				ToAccountID:   5,
				Amount:        1,
			},
			want: ErrIdenticalAccounts,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ExecuteTransfer(context.Background(), tt.dto)
			if err != tt.want {
				t.Fatalf("expected error %v, got %v", tt.want, err)
			}
		})
	}
}

package query

import (
	"testing"

	"pkg.akt.dev/go/testutil"
)

func TestParseGroupPath(t *testing.T) {
	owner := testutil.AccAddress(t)

	tests := []struct {
		name    string
		parts   []string
		wantErr bool
	}{
		{
			name:    "valid path",
			parts:   []string{owner.String(), "100", "1"},
			wantErr: false,
		},
		{
			name:    "too few parts",
			parts:   []string{owner.String(), "100"},
			wantErr: true,
		},
		{
			name:    "empty parts",
			parts:   []string{},
			wantErr: true,
		},
		{
			name:    "single part",
			parts:   []string{owner.String()},
			wantErr: true,
		},
		{
			name:    "invalid dseq",
			parts:   []string{owner.String(), "invalid", "1"},
			wantErr: true,
		},
		{
			name:    "invalid gseq",
			parts:   []string{owner.String(), "100", "invalid"},
			wantErr: true,
		},
		{
			name:    "invalid owner address",
			parts:   []string{"invalidaddress", "100", "1"},
			wantErr: true,
		},
		{
			name:    "negative dseq",
			parts:   []string{owner.String(), "-100", "1"},
			wantErr: true,
		},
		{
			name:    "negative gseq",
			parts:   []string{owner.String(), "100", "-1"},
			wantErr: true,
		},
		{
			name:    "extra parts are ignored",
			parts:   []string{owner.String(), "100", "1", "extra", "parts"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGroupPath(tt.parts)

			if tt.wantErr {
				if err == nil {
					t.Error("ParseGroupPath() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseGroupPath() unexpected error: %v", err)
				return
			}

			if got.Owner != owner.String() {
				t.Errorf("ParseGroupPath() Owner = %v, want %v", got.Owner, owner.String())
			}
			if got.DSeq != 100 {
				t.Errorf("ParseGroupPath() DSeq = %v, want %v", got.DSeq, 100)
			}
			if got.GSeq != 1 {
				t.Errorf("ParseGroupPath() GSeq = %v, want %v", got.GSeq, 1)
			}
		})
	}
}

func TestParseGroupPathValues(t *testing.T) {
	owner := testutil.AccAddress(t)

	tests := []struct {
		name         string
		parts        []string
		expectedDSeq uint64
		expectedGSeq uint32
	}{
		{
			name:         "standard values",
			parts:        []string{owner.String(), "100", "1"},
			expectedDSeq: 100,
			expectedGSeq: 1,
		},
		{
			name:         "zero values",
			parts:        []string{owner.String(), "0", "0"},
			expectedDSeq: 0,
			expectedGSeq: 0,
		},
		{
			name:         "large dseq",
			parts:        []string{owner.String(), "999999999", "5"},
			expectedDSeq: 999999999,
			expectedGSeq: 5,
		},
		{
			name:         "large gseq",
			parts:        []string{owner.String(), "1", "4294967295"},
			expectedDSeq: 1,
			expectedGSeq: 4294967295,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGroupPath(tt.parts)
			if err != nil {
				t.Errorf("ParseGroupPath() unexpected error: %v", err)
				return
			}

			if got.DSeq != tt.expectedDSeq {
				t.Errorf("ParseGroupPath() DSeq = %v, want %v", got.DSeq, tt.expectedDSeq)
			}
			if got.GSeq != tt.expectedGSeq {
				t.Errorf("ParseGroupPath() GSeq = %v, want %v", got.GSeq, tt.expectedGSeq)
			}
		})
	}
}

